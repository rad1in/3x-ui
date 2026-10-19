// Package webauthn verifies WebAuthn registration and assertion ceremonies
// for the panel's passkeys. Attestation statements are not checked: the panel
// trusts a key on first use and never gates on authenticator make or model.
package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
)

// COSE algorithm identifiers the panel offers and verifies.
const (
	AlgES256 = -7
	AlgEdDSA = -8
	AlgRS256 = -257
)

const (
	FlagUserPresent  = 0x01
	FlagUserVerified = 0x04
	FlagAttested     = 0x40

	typeCreate = "webauthn.create"
	typeGet    = "webauthn.get"
)

var (
	ErrMalformed         = errors.New("webauthn: malformed response")
	ErrWrongType         = errors.New("webauthn: wrong ceremony type")
	ErrChallengeMismatch = errors.New("webauthn: challenge mismatch")
	ErrOriginMismatch    = errors.New("webauthn: origin mismatch")
	ErrRPIDMismatch      = errors.New("webauthn: relying party ID hash mismatch")
	ErrUserNotPresent    = errors.New("webauthn: user presence flag not set")
	ErrUserNotVerified   = errors.New("webauthn: user verification required")
	ErrBadSignature      = errors.New("webauthn: signature check failed")
	ErrCounterRegression = errors.New("webauthn: signature counter did not advance")
	ErrUnsupportedKey    = errors.New("webauthn: unsupported credential key")
)

// Expect is what the server demands of a ceremony it started.
type Expect struct {
	Challenge []byte
	Origin    string
	RPID      string
	// RequireUV rejects authenticators that did not verify the user (PIN, biometric).
	RequireUV bool
}

// Credential is what a registration yields and an assertion later needs.
type Credential struct {
	ID        []byte
	PublicKey []byte // COSE_Key, stored as the authenticator sent it
	SignCount uint32
}

type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

type authData struct {
	rpIDHash  [32]byte
	flags     byte
	signCount uint32
	rest      []byte
}

func parseAuthData(b []byte) (*authData, error) {
	if len(b) < 37 {
		return nil, ErrMalformed
	}
	a := &authData{flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37]), rest: b[37:]}
	copy(a.rpIDHash[:], b[:32])
	return a, nil
}

func checkClientData(raw []byte, wantType string, exp Expect) error {
	var cd clientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return ErrMalformed
	}
	if cd.Type != wantType {
		return ErrWrongType
	}
	if cd.CrossOrigin {
		return ErrOriginMismatch
	}
	got, err := b64Decode(cd.Challenge)
	if err != nil || subtle.ConstantTimeCompare(got, exp.Challenge) != 1 {
		return ErrChallengeMismatch
	}
	if cd.Origin != exp.Origin {
		return ErrOriginMismatch
	}
	return nil
}

func checkAuthData(a *authData, exp Expect) error {
	want := sha256.Sum256([]byte(exp.RPID))
	if subtle.ConstantTimeCompare(a.rpIDHash[:], want[:]) != 1 {
		return ErrRPIDMismatch
	}
	if a.flags&FlagUserPresent == 0 {
		return ErrUserNotPresent
	}
	if exp.RequireUV && a.flags&FlagUserVerified == 0 {
		return ErrUserNotVerified
	}
	return nil
}

// VerifyRegistration checks a navigator.credentials.create() response and
// returns the new credential.
func VerifyRegistration(clientDataJSON, attestationObject []byte, exp Expect) (*Credential, error) {
	if err := checkClientData(clientDataJSON, typeCreate, exp); err != nil {
		return nil, err
	}
	decoded, _, err := decodeCBOR(attestationObject)
	if err != nil {
		return nil, ErrMalformed
	}
	obj, _ := decoded.(map[any]any)
	raw, _ := obj["authData"].([]byte)
	ad, err := parseAuthData(raw)
	if err != nil {
		return nil, err
	}
	if err := checkAuthData(ad, exp); err != nil {
		return nil, err
	}
	if ad.flags&FlagAttested == 0 || len(ad.rest) < 18 {
		return nil, ErrMalformed
	}
	idLen := int(binary.BigEndian.Uint16(ad.rest[16:18]))
	if idLen == 0 || idLen > 1023 || len(ad.rest) < 18+idLen {
		return nil, ErrMalformed
	}
	id := append([]byte(nil), ad.rest[18:18+idLen]...)
	keyBytes := ad.rest[18+idLen:]
	_, used, err := decodeCBOR(keyBytes)
	if err != nil {
		return nil, ErrMalformed
	}
	cose := append([]byte(nil), keyBytes[:used]...)
	if _, _, err := parsePublicKey(cose); err != nil {
		return nil, err
	}
	return &Credential{ID: id, PublicKey: cose, SignCount: ad.signCount}, nil
}

// VerifyAssertion checks a navigator.credentials.get() response against the
// stored credential and returns the authenticator's new signature counter and
// whether it verified the user. A counter that is non-zero must advance.
func VerifyAssertion(clientDataJSON, authenticatorData, signature []byte, publicKey []byte, storedCount uint32, exp Expect) (newCount uint32, userVerified bool, err error) {
	if err := checkClientData(clientDataJSON, typeGet, exp); err != nil {
		return 0, false, err
	}
	ad, err := parseAuthData(authenticatorData)
	if err != nil {
		return 0, false, err
	}
	if err := checkAuthData(ad, exp); err != nil {
		return 0, false, err
	}
	key, _, err := parsePublicKey(publicKey)
	if err != nil {
		return 0, false, err
	}
	hash := sha256.Sum256(clientDataJSON)
	signed := append(append([]byte(nil), authenticatorData...), hash[:]...)
	if !verifySignature(key, signed, signature) {
		return 0, false, ErrBadSignature
	}
	if (ad.signCount != 0 || storedCount != 0) && ad.signCount <= storedCount {
		return 0, false, ErrCounterRegression
	}
	return ad.signCount, ad.flags&FlagUserVerified != 0, nil
}

func verifySignature(key any, signed, sig []byte) bool {
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		digest := sha256.Sum256(signed)
		return ecdsa.VerifyASN1(k, digest[:], sig)
	case ed25519.PublicKey:
		return ed25519.Verify(k, signed, sig)
	case *rsa.PublicKey:
		digest := sha256.Sum256(signed)
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, digest[:], sig) == nil
	}
	return false
}

func parsePublicKey(cose []byte) (any, int, error) {
	decoded, _, err := decodeCBOR(cose)
	m, ok := decoded.(map[any]any)
	if err != nil || !ok {
		return nil, 0, ErrUnsupportedKey
	}
	kty, _ := m[int64(1)].(int64)
	alg, _ := m[int64(3)].(int64)
	switch {
	case kty == 2 && alg == AlgES256:
		crv, _ := m[int64(-1)].(int64)
		x, _ := m[int64(-2)].([]byte)
		y, _ := m[int64(-3)].([]byte)
		if crv != 1 || len(x) != 32 || len(y) != 32 {
			return nil, 0, ErrUnsupportedKey
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			return nil, 0, ErrUnsupportedKey
		}
		return pub, AlgES256, nil
	case kty == 1 && alg == AlgEdDSA:
		crv, _ := m[int64(-1)].(int64)
		x, _ := m[int64(-2)].([]byte)
		if crv != 6 || len(x) != ed25519.PublicKeySize {
			return nil, 0, ErrUnsupportedKey
		}
		return ed25519.PublicKey(x), AlgEdDSA, nil
	case kty == 3 && alg == AlgRS256:
		n, _ := m[int64(-1)].([]byte)
		e, _ := m[int64(-2)].([]byte)
		if len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return nil, 0, ErrUnsupportedKey
		}
		exp := int(new(big.Int).SetBytes(e).Int64())
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}, AlgRS256, nil
	}
	return nil, 0, ErrUnsupportedKey
}
