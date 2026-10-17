package decoy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func serve(t *testing.T, site Site, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/real", func(c *gin.Context) { c.String(http.StatusOK, "real") })
	if h := site.Handler(); h != nil {
		engine.NoRoute(h)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestNginxDecoyLooksLikeAFreshNginx(t *testing.T) {
	site := Site{Mode: ModeNginx}
	cases := []struct {
		method, path string
		code         int
		body         string
	}{
		{http.MethodGet, "/", http.StatusOK, "<title>Welcome to nginx!</title>"},
		{http.MethodGet, "/wp-login.php", http.StatusNotFound, "<center><h1>404 Not Found</h1></center>"},
		{http.MethodPost, "/", http.StatusMethodNotAllowed, "<center><h1>405 Not Allowed</h1></center>"},
		{http.MethodHead, "/", http.StatusOK, ""},
	}
	for _, c := range cases {
		w := serve(t, site, c.method, c.path)
		if w.Code != c.code || w.Header().Get("Server") != "nginx" || !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s %s = %d, Server %q, body %q; want %d with %q", c.method, c.path, w.Code, w.Header().Get("Server"), w.Body.String(), c.code, c.body)
		}
		if c.method == http.MethodHead && w.Body.Len() != 0 {
			t.Errorf("HEAD carried a body: %q", w.Body.String())
		}
	}
	if w := serve(t, site, http.MethodGet, "/real"); w.Body.String() != "real" {
		t.Fatalf("a real route was shadowed: %q", w.Body.String())
	}
}

func TestRedirectAndCustomDecoys(t *testing.T) {
	w := serve(t, Site{Mode: ModeRedirect, Target: "https://www.example.org/"}, http.MethodGet, "/anything")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://www.example.org/" {
		t.Fatalf("redirect: %d to %q", w.Code, w.Header().Get("Location"))
	}
	w = serve(t, Site{Mode: ModeHTML, HTML: "<h1>Bakery</h1>"}, http.MethodGet, "/x")
	if w.Code != http.StatusOK || w.Body.String() != "<h1>Bakery</h1>" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("custom page: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

// A decoy that cannot work falls back to the plain 404 instead of a broken answer.
func TestUnusableDecoyIsOff(t *testing.T) {
	for _, site := range []Site{{}, {Mode: ModeRedirect, Target: "javascript:alert(1)"}, {Mode: ModeRedirect}, {Mode: ModeHTML, HTML: "  "}, {Mode: "bogus"}} {
		if site.Handler() != nil {
			t.Errorf("%+v produced a handler", site)
		}
	}
}
