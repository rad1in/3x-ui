package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
)

// With a secret base path and a panel domain, a scanner probing the panel
// port gets a stock nginx instead of a 404 or 403 that fingerprints 3x-ui.
func TestPanelAnswersStrangersWithTheDecoy(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))
	for k, v := range map[string]string{"webBasePath": "/s3cret/", "webDomain": "panel.example.com", "decoyMode": "nginx"} {
		if err := database.GetDB().Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	previous := global.GetWebServer()
	s := NewServer()
	s.cron = cron.New(cron.WithLocation(time.Local), cron.WithSeconds())
	global.SetWebServer(s)
	t.Cleanup(func() {
		s.cancel()
		global.SetWebServer(previous)
	})
	engine, err := s.initRouter()
	if err != nil {
		t.Fatal(err)
	}
	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		engine.ServeHTTP(w, req)
		if w.Header().Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatalf("%s: gzip body: %v", url, err)
			}
			plain, _ := io.ReadAll(zr)
			w.Body = bytes.NewBuffer(plain)
		}
		return w
	}
	if w := get("http://panel.example.com/"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Welcome to nginx!") {
		t.Fatalf("root outside the base path: %d %q", w.Code, w.Body.String())
	}
	if w := get("http://203.0.113.9/s3cret/"); w.Code != http.StatusNotFound || w.Header().Get("Server") != "nginx" {
		t.Fatalf("request by IP: %d %q; want nginx's 404, not a telltale 403", w.Code, w.Body.String())
	}
	for _, url := range []string{"http://panel.example.com/x", "http://203.0.113.9/"} {
		w := get(url)
		for _, giveaway := range []string{"Content-Security-Policy", "X-Frame-Options", "Referrer-Policy"} {
			if v := w.Header().Get(giveaway); v != "" {
				t.Errorf("%s decoy carries the panel's %s: %q", url, giveaway, v)
			}
		}
	}
	if w := get("http://panel.example.com/s3cret/"); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "nginx") {
		t.Fatalf("the login page was shadowed: %d", w.Code)
	}
}
