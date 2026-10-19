package session

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestSetLoginUserStoresOnlyUserIDAndASessionId(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, cookie.NewStore([]byte("01234567890123456789012345678901"))))
	router.GET("/", func(c *gin.Context) {
		if err := SetLoginUser(c, &model.User{Id: 7, Username: "admin", Password: "hash"}); err != nil {
			t.Fatal(err)
		}
		got := sessions.Default(c).Get(loginUserKey)
		if got != 7 {
			t.Fatalf("stored session payload = %#v, want user id only", got)
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSessionUserIDSupportsLegacyUserPayload(t *testing.T) {
	id, ok := sessionUserID(model.User{Id: 11, Username: "admin", Password: "hash"})
	if !ok || id != 11 {
		t.Fatalf("legacy session payload resolved to (%d, %v), want (11, true)", id, ok)
	}
	id, ok = sessionUserID(&model.User{Id: 12, Username: "admin", Password: "hash"})
	if !ok || id != 12 {
		t.Fatalf("legacy pointer session payload resolved to (%d, %v), want (12, true)", id, ok)
	}
}

type sessionRig struct {
	server *httptest.Server
	http   *http.Client
	userId int
}

// newSessionRig serves /login (signs the first user in), /me (who is logged in)
// and /legacy-login (the cookie an old build issued: user id and epoch, no session id).
func newSessionRig(t *testing.T) *sessionRig {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, cookie.NewStore([]byte("01234567890123456789012345678901"))))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, &user); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/legacy-login", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set(loginUserKey, user.Id)
		s.Set(loginEpochKey, user.LoginEpoch)
		_ = s.Save()
		c.Status(http.StatusNoContent)
	})
	router.GET("/me", func(c *gin.Context) {
		if GetLoginUser(c) == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &sessionRig{server: srv, http: &http.Client{Jar: jar}, userId: user.Id}
}

func (r *sessionRig) status(t *testing.T, path string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, r.server.URL+path, nil)
	req.Header.Set("User-Agent", "rig-browser/1.0")
	resp, err := r.http.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func rows(t *testing.T) []model.AdminSession {
	t.Helper()
	var out []model.AdminSession
	if err := database.GetDB().Find(&out).Error; err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	return out
}

func TestLoginRecordsASessionAndDeletingItSignsTheBrowserOut(t *testing.T) {
	rig := newSessionRig(t)
	if got := rig.status(t, "/login"); got != http.StatusNoContent {
		t.Fatalf("login: %d", got)
	}
	got := rows(t)
	if len(got) != 1 || got[0].UserId != rig.userId || got[0].UserAgent != "rig-browser/1.0" || got[0].LastSeen == 0 {
		t.Fatalf("session rows = %+v, want one for user %d with the browser's user agent", got, rig.userId)
	}
	if rig.status(t, "/me") != http.StatusNoContent {
		t.Fatal("a fresh session was refused")
	}
	if err := database.GetDB().Delete(&model.AdminSession{}, "id = ?", got[0].Id).Error; err != nil {
		t.Fatal(err)
	}
	if rig.status(t, "/me") != http.StatusUnauthorized {
		t.Fatal("a revoked session still passes; the cookie alone must not be enough")
	}
}

func TestACookieFromBeforeSessionTrackingIsRegisteredNotRejected(t *testing.T) {
	rig := newSessionRig(t)
	rig.status(t, "/legacy-login")
	if rig.status(t, "/me") != http.StatusNoContent {
		t.Fatal("upgrading logged every admin out")
	}
	if got := rows(t); len(got) != 1 {
		t.Fatalf("session rows = %+v, want the legacy cookie registered so it can be revoked", got)
	}
	if err := database.GetDB().Where("1 = 1").Delete(&model.AdminSession{}).Error; err != nil {
		t.Fatal(err)
	}
	if rig.status(t, "/me") != http.StatusUnauthorized {
		t.Fatal("a registered legacy session could not be revoked")
	}
}
