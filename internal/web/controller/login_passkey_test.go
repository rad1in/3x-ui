package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn/webauthntest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const passkeyUV = webauthn.FlagUserPresent | webauthn.FlagUserVerified

func (h *loginHarness) origin() (origin, rpID string) {
	u, _ := url.Parse(h.srv.URL)
	return h.srv.URL, u.Hostname()
}

func (h *loginHarness) postJSON(path string, body any) loginReply {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", h.csrf)
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out loginReply
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		h.t.Fatalf("POST %s: %v", path, err)
	}
	return out
}

func e64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func decodeChallenge(t *testing.T, options any) []byte {
	t.Helper()
	m, _ := options.(map[string]any)
	raw, err := base64.RawURLEncoding.DecodeString(m["challenge"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// enrol registers a passkey for the seeded admin straight through the service.
func (h *loginHarness) enrol(auth *webauthntest.SoftAuth) {
	h.t.Helper()
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		h.t.Fatal(err)
	}
	origin, rpID := h.origin()
	opts, cid, err := loginPasskeys.BeginRegistration(&user, origin, rpID)
	if err != nil {
		h.t.Fatal(err)
	}
	cdj, att := auth.Register(decodeChallenge(h.t, opts), origin, rpID, webauthn.FlagUserPresent)
	var cred service.PasskeyCredentialJSON
	cred.ID = e64(auth.CredID)
	cred.Response.ClientDataJSON, cred.Response.AttestationObject = e64(cdj), e64(att)
	if _, err := loginPasskeys.FinishRegistration(&user, cid, "test", cred); err != nil {
		h.t.Fatal(err)
	}
}

func (h *loginHarness) credential(auth *webauthntest.SoftAuth, options any, flags byte) map[string]any {
	origin, rpID := h.origin()
	cdj, ad, sig := auth.Assert(decodeChallenge(h.t, options), origin, rpID, flags)
	return map[string]any{"id": e64(auth.CredID), "response": map[string]string{
		"clientDataJSON": e64(cdj), "authenticatorData": e64(ad), "signature": e64(sig),
	}}
}

func saveSecurity(t *testing.T, cfg service.SecurityConfig) {
	t.Helper()
	if err := (&service.SecurityConfigService{}).Save(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordAloneIsNotEnoughWithPasskeySecondFactor(t *testing.T) {
	h := newLoginHarness(t)
	saveSecurity(t, service.SecurityConfig{PasskeySecondFactor: true})
	auth := webauthntest.New(t, webauthn.AlgES256)
	h.enrol(auth)

	first := h.loginReply(credentials(nil))
	if first.Success || first.Obj["passkeyRequired"] != true {
		t.Fatalf("first step = %+v, want a passkey to be asked for", first)
	}
	options, cid := first.Obj["passkeyOptions"], first.Obj["passkeyChallengeId"].(string)

	stranger := webauthntest.New(t, webauthn.AlgES256)
	forged, _ := json.Marshal(h.credential(stranger, options, webauthn.FlagUserPresent))
	if h.login(credentials(url.Values{"passkeyChallengeId": {cid}, "passkeyCredential": {string(forged)}})) {
		t.Fatal("a passkey nobody registered logged in")
	}

	// The failed attempt burned that ceremony; a fresh one with the real key works.
	second := h.loginReply(credentials(nil))
	good, _ := json.Marshal(h.credential(auth, second.Obj["passkeyOptions"], webauthn.FlagUserPresent))
	if !h.login(credentials(url.Values{"passkeyChallengeId": {second.Obj["passkeyChallengeId"].(string)}, "passkeyCredential": {string(good)}})) {
		t.Fatal("the registered passkey was refused")
	}
}

func TestPasskeySecondFactorDoesNotLockOutAdminsWithoutPasskeys(t *testing.T) {
	h := newLoginHarness(t)
	saveSecurity(t, service.SecurityConfig{PasskeySecondFactor: true})
	if !h.login(credentials(nil)) {
		t.Fatal("an admin with no passkey was asked for one and locked out")
	}
}

func TestMadeUpPasskeyCeremonyDoesNotSkipTheCaptcha(t *testing.T) {
	h := newLoginHarness(t)
	saveSecurity(t, service.SecurityConfig{PasskeySecondFactor: true, CaptchaEnable: true})

	// Only a ceremony opened after a solved captcha may stand in for it.
	if h.login(credentials(url.Values{"passkeyChallengeId": {"made-up"}})) {
		t.Fatal("login without captcha succeeded")
	}
}

func TestPasswordlessPasskeyLogin(t *testing.T) {
	h := newLoginHarness(t)
	auth := webauthntest.New(t, webauthn.AlgES256)
	h.enrol(auth)

	t.Run("refused while the option is off", func(t *testing.T) {
		if r := h.postJSON("/passkey/login/begin", map[string]any{}); r.Success {
			t.Fatalf("begin worked with the option off: %+v", r)
		}
	})

	saveSecurity(t, service.SecurityConfig{PasskeyLoginEnable: true})
	finish := func(flags byte, twoFactor string) loginReply {
		begin := h.postJSON("/passkey/login/begin", map[string]any{})
		if !begin.Success {
			t.Fatalf("begin: %+v", begin)
		}
		return h.postJSON("/passkey/login/finish", map[string]any{
			"challengeId":   begin.Obj["challengeId"],
			"credential":    h.credential(auth, begin.Obj["options"], flags),
			"twoFactorCode": twoFactor,
		})
	}

	t.Run("needs a user-verifying authenticator", func(t *testing.T) {
		if r := finish(webauthn.FlagUserPresent, ""); r.Success {
			t.Fatal("a passkey that did not verify the user logged in")
		}
	})
	t.Run("owner two-factor code is still required", func(t *testing.T) {
		svc := service.SettingService{}
		if err := svc.SetTwoFactorEnable(true); err != nil {
			t.Fatal(err)
		}
		if err := svc.SetTwoFactorToken("JBSWY3DPEHPK3PXP"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = svc.SetTwoFactorEnable(false) }()
		if r := finish(passkeyUV, ""); r.Success {
			t.Fatal("passkey login bypassed the panel two-factor code")
		}
	})
	t.Run("signs in", func(t *testing.T) {
		if r := finish(passkeyUV, ""); !r.Success {
			t.Fatalf("login refused: %+v", r)
		}
	})
}
