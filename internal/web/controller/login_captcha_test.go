package controller

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type loginHarness struct {
	t      *testing.T
	srv    *httptest.Server
	client *http.Client
	csrf   string
}

func newLoginHarness(t *testing.T) *loginHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))
	hash, err := crypto.HashPasswordAsBcrypt("s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.User{}).Where("1 = 1").Update("password", hash).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("login-captcha-test-secret"))))
	NewIndexController(engine.Group("/"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	h := &loginHarness{t: t, srv: srv, client: &http.Client{Jar: jar}}
	var out struct{ Obj string }
	h.get("/csrf-token", &out)
	h.csrf = out.Obj
	return h
}

func (h *loginHarness) get(path string, into any) {
	h.t.Helper()
	resp, err := h.client.Get(h.srv.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
}

func (h *loginHarness) login(form url.Values) bool {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", h.csrf)
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Success bool }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		h.t.Fatal(err)
	}
	return out.Success
}

func credentials(extra url.Values) url.Values {
	form := url.Values{"username": {"admin"}, "password": {"s3cret-pass"}}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func enableCaptcha(t *testing.T) {
	t.Helper()
	if err := (&service.SecurityConfigService{}).Save(&service.SecurityConfig{CaptchaEnable: true}); err != nil {
		t.Fatal(err)
	}
}

func TestLoginWithoutCaptchaStillWorksWhenItIsOff(t *testing.T) {
	h := newLoginHarness(t)
	if !h.login(credentials(nil)) {
		t.Fatal("a plain login failed with the captcha switched off")
	}
}

func TestLoginNeedsASolvedCaptchaWhenItIsOn(t *testing.T) {
	h := newLoginHarness(t)
	enableCaptcha(t)

	if h.login(credentials(nil)) {
		t.Fatal("logged in with a correct password but no captcha")
	}
	ch, err := loginCaptcha.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if h.login(credentials(url.Values{"captchaId": {ch.ID}, "captchaAnswer": {"wrong"}})) {
		t.Fatal("logged in with a wrong captcha answer")
	}
	fresh, _ := loginCaptcha.Issue()
	if !h.login(credentials(url.Values{"captchaId": {fresh.ID}, "captchaAnswer": {fresh.Answer}})) {
		t.Fatal("a correct password and captcha was refused")
	}
}

func TestACaptchaCannotBeReplayedAcrossLogins(t *testing.T) {
	h := newLoginHarness(t)
	enableCaptcha(t)
	ch, _ := loginCaptcha.Issue()
	bad := credentials(url.Values{"password": {"nope"}, "captchaId": {ch.ID}, "captchaAnswer": {ch.Answer}})
	if h.login(bad) {
		t.Fatal("a wrong password logged in")
	}
	good := credentials(url.Values{"captchaId": {ch.ID}, "captchaAnswer": {ch.Answer}})
	if h.login(good) {
		t.Fatal("a solved captcha was reused for a second attempt")
	}
}

func TestCaptchaEndpointOnlyIssuesWhenEnabled(t *testing.T) {
	h := newLoginHarness(t)
	var off struct {
		Obj struct {
			Enabled bool
			Id      string
			Image   string
		}
	}
	h.get("/captcha", &off)
	if off.Obj.Enabled || off.Obj.Id != "" || off.Obj.Image != "" {
		t.Fatalf("captcha offered while disabled: %+v", off.Obj)
	}
	enableCaptcha(t)
	var on struct {
		Obj struct {
			Enabled bool
			Id      string
			Image   string
		}
	}
	h.get("/captcha", &on)
	if !on.Obj.Enabled || on.Obj.Id == "" || !strings.HasPrefix(on.Obj.Image, "data:image/png;base64,") {
		t.Fatalf("captcha response = %+v", on.Obj)
	}
}
