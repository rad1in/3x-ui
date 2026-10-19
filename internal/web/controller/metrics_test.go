package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestMetricsEndpointAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "eternal.db"))
	engine := gin.New()
	NewMetricsController(engine.Group("/"))

	setSetting := func(key, value string) {
		t.Helper()
		row := model.Setting{Key: key}
		if err := database.GetDB().Where(`"key" = ?`, key).Assign(model.Setting{Value: value}).FirstOrCreate(&row).Error; err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	get := func(mutate func(*http.Request)) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		mutate(req)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec.Code
	}
	bearer := func(tok string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
	}
	basic := func(r *http.Request) { r.SetBasicAuth("prometheus", "s3cret-token") }
	none := func(*http.Request) {}

	if code := get(bearer("s3cret-token")); code != http.StatusNotFound {
		t.Fatalf("disabled endpoint answered %d, want 404", code)
	}

	setSetting("metricsEnable", "true")
	if code := get(bearer("s3cret-token")); code != http.StatusNotFound {
		t.Fatalf("enabled but token-less endpoint answered %d, want 404", code)
	}

	setSetting("metricsToken", "s3cret-token")
	for name, tc := range map[string]struct {
		mutate func(*http.Request)
		want   int
	}{
		"bearer":       {bearer("s3cret-token"), http.StatusOK},
		"basic":        {basic, http.StatusOK},
		"wrong bearer": {bearer("nope"), http.StatusUnauthorized},
		"no creds":     {none, http.StatusUnauthorized},
	} {
		if code := get(tc.mutate); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret-token")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Errorf("Content-Type = %q, want the Prometheus text type", ct)
	}
	if !strings.Contains(rec.Body.String(), "xui_panel_up 1\n") {
		t.Errorf("body lacks xui_panel_up:\n%s", rec.Body.String())
	}
}
