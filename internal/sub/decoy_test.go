package sub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func setSubSettings(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := database.GetDB().Where("key = ?", k).Delete(&model.Setting{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

// A scanner probing the subscription port sees a stock nginx, both on paths
// that match nothing and when it reaches the server by IP instead of the domain.
func TestSubServerAnswersStrangersWithTheDecoy(t *testing.T) {
	initSubDB(t)
	seedInfoEndpointSub(t, "decoy-sub", "decoy@example.com")
	setSubSettings(t, map[string]string{"subPath": "/sub/", "subDomain": "sub.example.com", "decoyMode": "nginx"})
	router, err := (&Server{}).initRouter()
	if err != nil {
		t.Fatal(err)
	}
	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w
	}
	if w := get("http://sub.example.com/admin"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "<center>nginx</center>") {
		t.Fatalf("unknown path: %d %q", w.Code, w.Body.String())
	}
	if w := get("http://198.51.100.7/"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Welcome to nginx!") {
		t.Fatalf("request by IP: %d %q; want the nginx welcome page, not a telltale 403", w.Code, w.Body.String())
	}
	if w := get("http://sub.example.com/sub/decoy-sub"); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "nginx") {
		t.Fatalf("the real subscription was shadowed: %d %q", w.Code, w.Body.String())
	}
}
