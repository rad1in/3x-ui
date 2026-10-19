package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestFixedWindowLimiter(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := newFixedWindowLimiter(2, func() time.Time { return now })
	for i := range 2 {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("request %d inside the limit was refused", i+1)
		}
	}
	now = now.Add(20 * time.Second)
	ok, wait := l.allow("a")
	if ok || wait != 40*time.Second {
		t.Fatalf("third request = %v, retry after %v; want refused with 40s left in the window", ok, wait)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("one key's quota must not spend another's")
	}
	now = now.Add(40 * time.Second)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("a new window must reset the count")
	}
}

func TestSubscriptionRateLimit(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "s1", "rl1", 4483, 1, `{"network":"tcp","security":"none"}`)
	seedSubInbound(t, "s2", "rl2", 4484, 1, `{"network":"tcp","security":"none"}`)
	gin.SetMode(gin.TestMode)
	get := func(router *gin.Engine, path, remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://sub.example.com"+path, nil)
		req.RemoteAddr = remote
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}
	newRouter := func(perIP, perSub int) *gin.Engine {
		router := gin.New()
		NewSUBController(router.Group("/"), WithSUBJsonEnabled(true), WithSUBRateLimits(perIP, perSub))
		return router
	}

	t.Run("per IP", func(t *testing.T) {
		router := newRouter(2, 0)
		get(router, "/sub/s1", "198.51.100.7:1000")
		get(router, "/json/s2", "198.51.100.7:1001")
		resp := get(router, "/sub/s2", "198.51.100.7:1002")
		if resp.Code != http.StatusTooManyRequests || resp.Header().Get("Retry-After") == "" {
			t.Fatalf("third request from one IP = %d (Retry-After %q), want 429 with Retry-After", resp.Code, resp.Header().Get("Retry-After"))
		}
		if resp := get(router, "/sub/s2", "198.51.100.8:1000"); resp.Code != http.StatusOK {
			t.Fatalf("another IP = %d, want 200", resp.Code)
		}
	})

	t.Run("per subscription", func(t *testing.T) {
		router := newRouter(0, 1)
		get(router, "/sub/s1", "198.51.100.7:1000")
		if resp := get(router, "/json/s1", "203.0.113.9:1000"); resp.Code != http.StatusTooManyRequests {
			t.Fatalf("second fetch of s1 from any IP = %d, want 429", resp.Code)
		}
		if resp := get(router, "/sub/s2", "198.51.100.7:1000"); resp.Code != http.StatusOK {
			t.Fatalf("another subscription = %d, want 200", resp.Code)
		}
	})

	t.Run("off by default", func(t *testing.T) {
		router := newRouter(0, 0)
		for range 5 {
			if resp := get(router, "/sub/s1", "198.51.100.7:1000"); resp.Code != http.StatusOK {
				t.Fatalf("unlimited server answered %d", resp.Code)
			}
		}
	})
}
