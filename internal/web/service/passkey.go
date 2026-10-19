package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn"
)

const (
	passkeyCeremonyTTL  = 5 * time.Minute
	passkeyMaxCeremonys = 2000
	passkeyMaxPerUser   = 20
	passkeyMaxIDBytes   = 255
	passkeyMaxNameRunes = 64
)

var (
	ErrPasskeyCeremony = errors.New("passkey ceremony expired or unknown")
	ErrPasskeyUnknown  = errors.New("passkey not recognised")
	ErrPasskeyLimit    = errors.New("too many passkeys registered")
	ErrPasskeyDup      = errors.New("passkey already registered")
	ErrPasskeyUser     = errors.New("passkey belongs to another account")
)

type passkeyCeremony struct {
	register  bool
	userId    int // 0 for a discoverable-credential login
	challenge []byte
	origin    string
	rpID      string
	requireUV bool
	expires   time.Time
}

// PasskeyService runs WebAuthn ceremonies. Challenges live in memory, are
// single use and expire; a restart simply drops them.
type PasskeyService struct {
	mu        sync.Mutex
	ceremonys map[string]passkeyCeremony
	now       func() time.Time
}

func NewPasskeyService() *PasskeyService {
	return &PasskeyService{ceremonys: map[string]passkeyCeremony{}, now: time.Now}
}

// PasskeyCredentialJSON is a credential as the browser page serialises it:
// every binary field is unpadded base64url.
type PasskeyCredentialJSON struct {
	ID       string `json:"id"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	} `json:"response"`
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func userHandle(userId int) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(userId))
}

func (s *PasskeyService) start(c passkeyCeremony) (string, error) {
	id := make([]byte, 24)
	c.challenge = make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	if _, err := rand.Read(c.challenge); err != nil {
		return "", err
	}
	now := s.now()
	c.expires = now.Add(passkeyCeremonyTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.ceremonys {
		if now.After(v.expires) {
			delete(s.ceremonys, k)
		}
	}
	if len(s.ceremonys) >= passkeyMaxCeremonys {
		return "", ErrPasskeyCeremony
	}
	s.ceremonys[b64(id)] = c
	return b64(id), nil
}

// take removes and returns a ceremony, so every challenge is usable once.
func (s *PasskeyService) take(id string, register bool) (passkeyCeremony, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.ceremonys[id]
	delete(s.ceremonys, id)
	if !ok || s.now().After(c.expires) || c.register != register {
		return passkeyCeremony{}, ErrPasskeyCeremony
	}
	return c, nil
}

// HasPendingFor reports whether id names a live second-factor ceremony opened
// for this username, without using it up.
func (s *PasskeyService) HasPendingFor(id, username string) bool {
	s.mu.Lock()
	c, ok := s.ceremonys[id]
	live := ok && !c.register && c.userId != 0 && !s.now().After(c.expires)
	s.mu.Unlock()
	if !live {
		return false
	}
	var n int64
	err := database.GetDB().Model(&model.User{}).Where("id = ? AND username = ?", c.userId, username).Count(&n).Error
	return err == nil && n == 1
}

func (s *PasskeyService) List(userId int) ([]model.Passkey, error) {
	out := []model.Passkey{}
	err := database.GetDB().Where("user_id = ?", userId).Order("id ASC").Find(&out).Error
	return out, err
}

func (s *PasskeyService) Delete(userId, id int) error {
	res := database.GetDB().Where("id = ? AND user_id = ?", id, userId).Delete(&model.Passkey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPasskeyUnknown
	}
	return nil
}

func (s *PasskeyService) HasAny(userId int) (bool, error) {
	var n int64
	err := database.GetDB().Model(&model.Passkey{}).Where("user_id = ?", userId).Count(&n).Error
	return n > 0, err
}

// BeginRegistration returns the create() options and the ceremony id.
func (s *PasskeyService) BeginRegistration(user *model.User, origin, rpID string) (map[string]any, string, error) {
	existing, err := s.List(user.Id)
	if err != nil {
		return nil, "", err
	}
	if len(existing) >= passkeyMaxPerUser {
		return nil, "", ErrPasskeyLimit
	}
	exclude := make([]map[string]any, 0, len(existing))
	for _, p := range existing {
		exclude = append(exclude, map[string]any{"type": "public-key", "id": p.CredentialId})
	}
	cid, err := s.start(passkeyCeremony{register: true, userId: user.Id, origin: origin, rpID: rpID})
	if err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	challenge := s.ceremonys[cid].challenge
	s.mu.Unlock()
	return map[string]any{
		"rp":        map[string]any{"id": rpID, "name": "Eternal"},
		"user":      map[string]any{"id": b64(userHandle(user.Id)), "name": user.Username, "displayName": user.Username},
		"challenge": b64(challenge),
		"pubKeyCredParams": []map[string]any{
			{"type": "public-key", "alg": webauthn.AlgES256},
			{"type": "public-key", "alg": webauthn.AlgEdDSA},
			{"type": "public-key", "alg": webauthn.AlgRS256},
		},
		"timeout":                int(passkeyCeremonyTTL / time.Millisecond),
		"excludeCredentials":     exclude,
		"authenticatorSelection": map[string]any{"residentKey": "preferred", "userVerification": "preferred"},
		"attestation":            "none",
	}, cid, nil
}

// FinishRegistration verifies the create() response and stores the passkey.
func (s *PasskeyService) FinishRegistration(user *model.User, cid, name string, cred PasskeyCredentialJSON) (*model.Passkey, error) {
	c, err := s.take(cid, true)
	if err != nil {
		return nil, err
	}
	if c.userId != user.Id {
		return nil, ErrPasskeyUser
	}
	cdj, err1 := base64.RawURLEncoding.DecodeString(cred.Response.ClientDataJSON)
	att, err2 := base64.RawURLEncoding.DecodeString(cred.Response.AttestationObject)
	if err1 != nil || err2 != nil {
		return nil, webauthn.ErrMalformed
	}
	verified, err := webauthn.VerifyRegistration(cdj, att, webauthn.Expect{Challenge: c.challenge, Origin: c.origin, RPID: c.rpID})
	if err != nil {
		return nil, err
	}
	if len(verified.ID) > passkeyMaxIDBytes {
		return nil, webauthn.ErrMalformed
	}
	if len([]rune(name)) > passkeyMaxNameRunes {
		name = string([]rune(name)[:passkeyMaxNameRunes])
	}
	if name == "" {
		name = "Passkey"
	}
	row := &model.Passkey{
		UserId:       user.Id,
		Name:         name,
		CredentialId: b64(verified.ID),
		PublicKey:    verified.PublicKey,
		SignCount:    int64(verified.SignCount),
		CreatedAt:    s.now().UnixMilli(),
	}
	var dup int64
	if err := database.GetDB().Model(&model.Passkey{}).Where("credential_id = ?", row.CredentialId).Count(&dup).Error; err != nil {
		return nil, err
	}
	if dup > 0 {
		return nil, ErrPasskeyDup
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// BeginLogin returns the get() options. userId 0 asks for a discoverable
// credential (passwordless); otherwise only that admin's passkeys are offered.
func (s *PasskeyService) BeginLogin(userId int, requireUV bool, origin, rpID string) (map[string]any, string, error) {
	allow := []map[string]any{}
	if userId != 0 {
		rows, err := s.List(userId)
		if err != nil {
			return nil, "", err
		}
		for _, p := range rows {
			allow = append(allow, map[string]any{"type": "public-key", "id": p.CredentialId})
		}
	}
	cid, err := s.start(passkeyCeremony{userId: userId, origin: origin, rpID: rpID, requireUV: requireUV})
	if err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	challenge := s.ceremonys[cid].challenge
	s.mu.Unlock()
	uv := "preferred"
	if requireUV {
		uv = "required"
	}
	return map[string]any{
		"challenge":        b64(challenge),
		"rpId":             rpID,
		"timeout":          int(passkeyCeremonyTTL / time.Millisecond),
		"userVerification": uv,
		"allowCredentials": allow,
	}, cid, nil
}

// FinishLogin verifies the get() response and returns the account it proves.
// A ceremony begun for one admin never authenticates another.
func (s *PasskeyService) FinishLogin(cid string, cred PasskeyCredentialJSON) (*model.User, error) {
	c, err := s.take(cid, false)
	if err != nil {
		return nil, err
	}
	cdj, e1 := base64.RawURLEncoding.DecodeString(cred.Response.ClientDataJSON)
	ad, e2 := base64.RawURLEncoding.DecodeString(cred.Response.AuthenticatorData)
	sig, e3 := base64.RawURLEncoding.DecodeString(cred.Response.Signature)
	if e1 != nil || e2 != nil || e3 != nil {
		return nil, webauthn.ErrMalformed
	}
	db := database.GetDB()
	var row model.Passkey
	if err := db.Where("credential_id = ?", cred.ID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPasskeyUnknown
		}
		return nil, err
	}
	if c.userId != 0 && row.UserId != c.userId {
		return nil, ErrPasskeyUser
	}
	if cred.Response.UserHandle != "" && cred.Response.UserHandle != b64(userHandle(row.UserId)) {
		return nil, ErrPasskeyUser
	}
	count, _, err := webauthn.VerifyAssertion(cdj, ad, sig, row.PublicKey, uint32(row.SignCount),
		webauthn.Expect{Challenge: c.challenge, Origin: c.origin, RPID: c.rpID, RequireUV: c.requireUV})
	if err != nil {
		return nil, err
	}
	// The counter update is conditional so two racing replays cannot both win.
	res := db.Model(&model.Passkey{}).Where("id = ? AND sign_count = ?", row.Id, row.SignCount).
		Updates(map[string]any{"sign_count": int64(count), "last_used_at": s.now().UnixMilli()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, webauthn.ErrCounterRegression
	}
	var user model.User
	if err := db.First(&user, row.UserId).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
