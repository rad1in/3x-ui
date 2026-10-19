// Package webauthntest is a software authenticator for tests: real keys, real signatures.
package webauthntest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn"
	"testing"
)

const (
	RPID   = "panel.example.com"
	Origin = "https://panel.example.com"
)

// SoftAuth is a software authenticator: it makes real keys and signs the
// same bytes a hardware one would.
type SoftAuth struct {
	t       *testing.T
	alg     int
	CredID  []byte
	COSE    []byte
	sign    func(msg []byte) []byte
	Counter uint32
}

func New(t *testing.T, alg int) *SoftAuth {
	t.Helper()
	s := &SoftAuth{t: t, alg: alg, CredID: RandBytes(t, 32)}
	switch alg {
	case webauthn.AlgES256:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		s.COSE = CBORMap(
			CBORInt(1), CBORInt(2), CBORInt(3), CBORInt(webauthn.AlgES256), CBORInt(-1), CBORInt(1),
			CBORInt(-2), CBORBytes(pad32(k.X.Bytes())), CBORInt(-3), CBORBytes(pad32(k.Y.Bytes())),
		)
		s.sign = func(msg []byte) []byte {
			d := sha256.Sum256(msg)
			sig, err := ecdsa.SignASN1(rand.Reader, k, d[:])
			if err != nil {
				t.Fatal(err)
			}
			return sig
		}
	case webauthn.AlgEdDSA:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		s.COSE = CBORMap(CBORInt(1), CBORInt(1), CBORInt(3), CBORInt(webauthn.AlgEdDSA), CBORInt(-1), CBORInt(6), CBORInt(-2), CBORBytes(pub))
		s.sign = func(msg []byte) []byte { return ed25519.Sign(priv, msg) }
	case webauthn.AlgRS256:
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		s.COSE = CBORMap(CBORInt(1), CBORInt(3), CBORInt(3), CBORInt(webauthn.AlgRS256), CBORInt(-1), CBORBytes(k.N.Bytes()), CBORInt(-2), CBORBytes([]byte{1, 0, 1}))
		s.sign = func(msg []byte) []byte {
			d := sha256.Sum256(msg)
			sig, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, d[:])
			if err != nil {
				t.Fatal(err)
			}
			return sig
		}
	}
	return s
}

func RandBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func pad32(b []byte) []byte { return append(make([]byte, 32-len(b)), b...) }

func ClientDataJSON(t *testing.T, typ string, challenge []byte, origin string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"type": typ, "challenge": base64.RawURLEncoding.EncodeToString(challenge), "origin": origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (s *SoftAuth) authData(rpID string, flags byte, attested bool) []byte {
	h := sha256.Sum256([]byte(rpID))
	out := append([]byte(nil), h[:]...)
	if attested {
		flags |= webauthn.FlagAttested
	}
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, s.Counter)
	if attested {
		out = append(out, make([]byte, 16)...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(s.CredID)))
		out = append(out, s.CredID...)
		out = append(out, s.COSE...)
	}
	return out
}

// register answers a create() call.
func (s *SoftAuth) Register(challenge []byte, origin, rpID string, flags byte) (cdj, attObj []byte) {
	cdj = ClientDataJSON(s.t, "webauthn.create", challenge, origin)
	attObj = CBORMap(
		CBORText("fmt"), CBORText("none"),
		CBORText("attStmt"), CBORMap(),
		CBORText("authData"), CBORBytes(s.authData(rpID, flags, true)),
	)
	return cdj, attObj
}

// assert answers a get() call; it bumps the counter first, like a real one.
func (s *SoftAuth) Assert(challenge []byte, origin, rpID string, flags byte) (cdj, ad, sig []byte) {
	s.Counter++
	return s.AssertWithCounter(challenge, origin, rpID, flags)
}

func (s *SoftAuth) AssertWithCounter(challenge []byte, origin, rpID string, flags byte) (cdj, ad, sig []byte) {
	cdj = ClientDataJSON(s.t, "webauthn.get", challenge, origin)
	ad = s.authData(rpID, flags, false)
	h := sha256.Sum256(cdj)
	return cdj, ad, s.sign(append(append([]byte(nil), ad...), h[:]...))
}

func cborHead(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	case n < 65536:
		return binary.BigEndian.AppendUint16([]byte{major<<5 | 25}, uint16(n))
	}
	return binary.BigEndian.AppendUint32([]byte{major<<5 | 26}, uint32(n))
}

func CBORInt(n int64) []byte {
	if n >= 0 {
		return cborHead(0, uint64(n))
	}
	return cborHead(1, uint64(-1-n))
}
func CBORBytes(b []byte) []byte { return append(cborHead(2, uint64(len(b))), b...) }
func CBORText(s string) []byte  { return append(cborHead(3, uint64(len(s))), s...) }

// cborMap takes already-encoded key and value items alternately.
func CBORMap(items ...[]byte) []byte {
	out := cborHead(5, uint64(len(items)/2))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}
