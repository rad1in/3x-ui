package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const fakeShopHTML = `<!doctype html><html><head><script type="module" src="/assets/shop-abc.js"></script><link rel="stylesheet" href="/assets/shop-abc.css"></head><body><div id="app"></div></body></html>`

func shopPageEngine(t *testing.T, enabled bool) *gin.Engine {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	gin.SetMode(gin.TestMode)
	previous := distFS
	SetDistFS(fstest.MapFS{"dist/shop.html": {Data: []byte(fakeShopHTML)}})
	t.Cleanup(func() { SetDistFS(previous) })
	if err := (&service.SalesConfigService{}).Save(&service.SalesConfig{PublicEnabled: enabled}); err != nil {
		t.Fatalf("config: %v", err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("base_path", "/secret-panel-path/") })
	NewShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) })
	return engine
}

func TestShopPageLoadsItsBundleWithoutRevealingThePanelBasePath(t *testing.T) {
	engine := shopPageEngine(t, true)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shop/", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `src="/shop/assets/shop-abc.js"`) || !strings.Contains(body, `href="/shop/assets/shop-abc.css"`) {
		t.Fatalf("page = %d %s, want assets under /shop/assets", w.Code, body)
	}
	if !strings.Contains(body, `window.X_UI_BASE_PATH="/shop/"`) || strings.Contains(body, "secret-panel-path") {
		t.Fatalf("page = %s, want the shop base path and no trace of the panel's", body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
}

func TestShopPageIsInvisibleWhileTheShopIsOff(t *testing.T) {
	engine := shopPageEngine(t, false)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shop/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("page = %d, want 404 while the shop is off", w.Code)
	}
}
