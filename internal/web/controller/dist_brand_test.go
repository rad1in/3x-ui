package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func serveLoginPage(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDistFS := distFS
	SetDistFS(fstest.MapFS{
		"dist/login.html": {Data: []byte(`<!doctype html><html><head></head><body>login</body></html>`)},
	})
	t.Cleanup(func() { SetDistFS(oldDistFS) })

	engine := gin.New()
	engine.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("brand-test-secret"))))
	engine.GET("/", func(c *gin.Context) { serveDistPage(c, "login.html") })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	return w.Body.String()
}

func TestServeDistPageInjectsBrandWithoutBreakingOutOfScript(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	for key, value := range map[string]string{
		"panelBrandName":  `</script><img src=x onerror=alert(1)>`,
		"panelBrandColor": "#0f6fe0",
	} {
		if err := database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}

	body := serveLoginPage(t)

	if !strings.Contains(body, `window.X_UI_BRAND={"name":"\u003c/script\u003e\u003cimg src=x onerror=alert(1)\u003e","color":"#0f6fe0","logo":""}`) {
		t.Fatalf("brand not injected as escaped JSON:\n%s", body)
	}
	if strings.Contains(body, "</script><img") {
		t.Fatalf("brand name broke out of the inline script:\n%s", body)
	}
}

func TestServeDistPageOmitsBrandWhenUnset(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))

	if body := serveLoginPage(t); strings.Contains(body, "X_UI_BRAND") {
		t.Fatalf("unset brand should inject nothing:\n%s", body)
	}
}
