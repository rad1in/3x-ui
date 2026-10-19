package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn/webauthntest"
)

func passkeyFixture(t *testing.T) (*PasskeyService, *model.User, *model.User) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	a := &model.User{Username: "alice", Password: "x", Role: model.AdminRoleOwner}
	b := &model.User{Username: "bob", Password: "x", Role: model.AdminRoleAdmin}
	if err := database.GetDB().Create(a).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(b).Error; err != nil {
		t.Fatal(err)
	}
	return NewPasskeyService(), a, b
}

func e64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func challengeOf(t *testing.T, opts map[string]any) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(opts["challenge"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func registerPasskey(t *testing.T, s *PasskeyService, u *model.User, auth *webauthntest.SoftAuth) *model.Passkey {
	t.Helper()
	opts, cid, err := s.BeginRegistration(u, webauthntest.Origin, webauthntest.RPID)
	if err != nil {
		t.Fatal(err)
	}
	cdj, att := auth.Register(challengeOf(t, opts), webauthntest.Origin, webauthntest.RPID, webauthn.FlagUserPresent|webauthn.FlagUserVerified)
	var cred PasskeyCredentialJSON
	cred.ID = e64(auth.CredID)
	cred.Response.ClientDataJSON = e64(cdj)
	cred.Response.AttestationObject = e64(att)
	row, err := s.FinishRegistration(u, cid, "key", cred)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func assertion(auth *webauthntest.SoftAuth, challenge []byte, origin string, flags byte, handle string) PasskeyCredentialJSON {
	cdj, ad, sig := auth.Assert(challenge, origin, webauthntest.RPID, flags)
	var cred PasskeyCredentialJSON
	cred.ID = e64(auth.CredID)
	cred.Response.ClientDataJSON = e64(cdj)
	cred.Response.AuthenticatorData = e64(ad)
	cred.Response.Signature = e64(sig)
	cred.Response.UserHandle = handle
	return cred
}

func TestPasskeyPasswordlessLoginAndCounterPersistence(t *testing.T) {
	s, alice, _ := passkeyFixture(t)
	auth := webauthntest.New(t, webauthn.AlgES256)
	registerPasskey(t, s, alice, auth)

	opts, cid, err := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
	if err != nil {
		t.Fatal(err)
	}
	flags := byte(webauthn.FlagUserPresent | webauthn.FlagUserVerified)
	cred := assertion(auth, challengeOf(t, opts), webauthntest.Origin, flags, e64(userHandle(alice.Id)))
	user, err := s.FinishLogin(cid, cred)
	if err != nil || user.Id != alice.Id {
		t.Fatalf("login: user=%v err=%v", user, err)
	}
	var row model.Passkey
	if err := database.GetDB().First(&row).Error; err != nil || row.SignCount != 1 || row.LastUsedAt == 0 {
		t.Fatalf("counter not persisted: %+v err=%v", row, err)
	}

	// A ceremony id is single use, so the same response cannot be replayed.
	if _, err := s.FinishLogin(cid, cred); !errors.Is(err, ErrPasskeyCeremony) {
		t.Fatalf("ceremony reuse: got %v", err)
	}
}

func TestPasskeyCounterRegressionIsRefusedAfterClone(t *testing.T) {
	s, alice, _ := passkeyFixture(t)
	auth := webauthntest.New(t, webauthn.AlgES256)
	registerPasskey(t, s, alice, auth)
	flags := byte(webauthn.FlagUserPresent | webauthn.FlagUserVerified)

	for range 2 {
		opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		if _, err := s.FinishLogin(cid, assertion(auth, challengeOf(t, opts), webauthntest.Origin, flags, "")); err != nil {
			t.Fatal(err)
		}
	}
	auth.Counter = 0 // a clone that never saw the later logins
	opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
	_, err := s.FinishLogin(cid, assertion(auth, challengeOf(t, opts), webauthntest.Origin, flags, ""))
	if !errors.Is(err, webauthn.ErrCounterRegression) {
		t.Fatalf("got %v", err)
	}
}

func TestPasskeyLoginRefusals(t *testing.T) {
	s, alice, bob := passkeyFixture(t)
	authA := webauthntest.New(t, webauthn.AlgES256)
	authB := webauthntest.New(t, webauthn.AlgES256)
	registerPasskey(t, s, alice, authA)
	registerPasskey(t, s, bob, authB)
	up := byte(webauthn.FlagUserPresent)
	uv := byte(webauthn.FlagUserPresent | webauthn.FlagUserVerified)

	t.Run("wrong origin", func(t *testing.T) {
		opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		_, err := s.FinishLogin(cid, assertion(authA, challengeOf(t, opts), "https://evil.example.com", uv, ""))
		if !errors.Is(err, webauthn.ErrOriginMismatch) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("wrong challenge", func(t *testing.T) {
		_, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		_, err := s.FinishLogin(cid, assertion(authA, []byte("not the challenge"), webauthntest.Origin, uv, ""))
		if !errors.Is(err, webauthn.ErrChallengeMismatch) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("passwordless needs user verification", func(t *testing.T) {
		opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		_, err := s.FinishLogin(cid, assertion(authA, challengeOf(t, opts), webauthntest.Origin, up, ""))
		if !errors.Is(err, webauthn.ErrUserNotVerified) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("second factor ceremony for alice rejects bob's key", func(t *testing.T) {
		opts, cid, _ := s.BeginLogin(alice.Id, false, webauthntest.Origin, webauthntest.RPID)
		_, err := s.FinishLogin(cid, assertion(authB, challengeOf(t, opts), webauthntest.Origin, up, ""))
		if !errors.Is(err, ErrPasskeyUser) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("user handle of another account", func(t *testing.T) {
		opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		_, err := s.FinishLogin(cid, assertion(authA, challengeOf(t, opts), webauthntest.Origin, uv, e64(userHandle(bob.Id))))
		if !errors.Is(err, ErrPasskeyUser) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("expired ceremony", func(t *testing.T) {
		opts, cid, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
		s.now = func() time.Time { return time.Now().Add(passkeyCeremonyTTL + time.Second) }
		defer func() { s.now = time.Now }()
		_, err := s.FinishLogin(cid, assertion(authA, challengeOf(t, opts), webauthntest.Origin, uv, ""))
		if !errors.Is(err, ErrPasskeyCeremony) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("registration ceremony cannot finish a login", func(t *testing.T) {
		_, cid, _ := s.BeginRegistration(alice, webauthntest.Origin, webauthntest.RPID)
		if _, err := s.FinishLogin(cid, assertion(authA, []byte("x"), webauthntest.Origin, uv, "")); !errors.Is(err, ErrPasskeyCeremony) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestPasskeyRegistrationAndManagement(t *testing.T) {
	s, alice, bob := passkeyFixture(t)
	auth := webauthntest.New(t, webauthn.AlgES256)
	row := registerPasskey(t, s, alice, auth)

	t.Run("same authenticator cannot register twice", func(t *testing.T) {
		opts, cid, _ := s.BeginRegistration(alice, webauthntest.Origin, webauthntest.RPID)
		var excluded []map[string]any
		raw, _ := json.Marshal(opts["excludeCredentials"])
		_ = json.Unmarshal(raw, &excluded)
		if len(excluded) != 1 || excluded[0]["id"] != row.CredentialId {
			t.Fatalf("existing credential not excluded: %v", excluded)
		}
		cdj, att := auth.Register(challengeOf(t, opts), webauthntest.Origin, webauthntest.RPID, webauthn.FlagUserPresent)
		var cred PasskeyCredentialJSON
		cred.Response.ClientDataJSON, cred.Response.AttestationObject = e64(cdj), e64(att)
		if _, err := s.FinishRegistration(alice, cid, "again", cred); !errors.Is(err, ErrPasskeyDup) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("ceremony begun by one admin cannot be finished by another", func(t *testing.T) {
		opts, cid, _ := s.BeginRegistration(alice, webauthntest.Origin, webauthntest.RPID)
		other := webauthntest.New(t, webauthn.AlgES256)
		cdj, att := other.Register(challengeOf(t, opts), webauthntest.Origin, webauthntest.RPID, webauthn.FlagUserPresent)
		var cred PasskeyCredentialJSON
		cred.Response.ClientDataJSON, cred.Response.AttestationObject = e64(cdj), e64(att)
		if _, err := s.FinishRegistration(bob, cid, "x", cred); !errors.Is(err, ErrPasskeyUser) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("delete is scoped to the owner of the passkey", func(t *testing.T) {
		if err := s.Delete(bob.Id, row.Id); !errors.Is(err, ErrPasskeyUnknown) {
			t.Fatalf("bob deleted alice's passkey: %v", err)
		}
		if err := s.Delete(alice.Id, row.Id); err != nil {
			t.Fatal(err)
		}
		if rows, _ := s.List(alice.Id); len(rows) != 0 {
			t.Fatalf("still listed: %v", rows)
		}
	})
}

func TestPasskeyPendingForSkipsCaptchaOnlyForTheCeremonyOwner(t *testing.T) {
	s, alice, bob := passkeyFixture(t)
	_, second, _ := s.BeginLogin(alice.Id, false, webauthntest.Origin, webauthntest.RPID)
	_, passwordless, _ := s.BeginLogin(0, true, webauthntest.Origin, webauthntest.RPID)
	_, reg, _ := s.BeginRegistration(alice, webauthntest.Origin, webauthntest.RPID)

	if !s.HasPendingFor(second, "alice") {
		t.Fatal("alice's own second-factor ceremony not recognised")
	}
	for name, c := range map[string][2]string{
		"another username":      {second, "bob"},
		"passwordless ceremony": {passwordless, "alice"},
		"registration ceremony": {reg, "alice"},
		"unknown id":            {"nope", "alice"},
	} {
		if s.HasPendingFor(c[0], c[1]) {
			t.Fatalf("%s: captcha would be skipped", name)
		}
	}
	_ = bob
}
