package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func allowlistEngine(list *string, rejected *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	InvalidateIPAllowlist()
	e := gin.New()
	e.Use(IPAllowlistMiddleware(
		func() string { return *list },
		func(c *gin.Context) string { return c.GetHeader("X-Test-IP") },
		func(c *gin.Context) { *rejected++; c.Status(http.StatusNotFound) },
	))
	e.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return e
}

func TestIPAllowlistLetsOnlyListedClientsIn(t *testing.T) {
	cases := []struct {
		name, list, ip string
		want           int
	}{
		{"empty list is off", "", "203.0.113.9", http.StatusOK},
		{"blank entries are off", " , ", "203.0.113.9", http.StatusOK},
		{"inside a cidr", "10.0.0.0/8, 192.0.2.0/24", "192.0.2.77", http.StatusOK},
		{"exact address", "198.51.100.4", "198.51.100.4", http.StatusOK},
		{"outside every entry", "10.0.0.0/8,198.51.100.4", "203.0.113.9", http.StatusNotFound},
		{"ipv6 cidr", "2001:db8::/32", "2001:db8::1", http.StatusOK},
		{"ipv4-mapped client matches the v4 entry", "192.0.2.0/24", "::ffff:192.0.2.5", http.StatusOK},
		{"unknown client address is refused", "192.0.2.0/24", "unknown", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list, rejected := tc.list, 0
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Test-IP", tc.ip)
			w := httptest.NewRecorder()
			allowlistEngine(&list, &rejected).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if (w.Code == http.StatusNotFound) != (rejected == 1) {
				t.Fatalf("reject handler ran %d times for status %d", rejected, w.Code)
			}
		})
	}
}

func TestIPAllowlistFollowsAChangedListWithoutRestart(t *testing.T) {
	list, rejected := "", 0
	e := allowlistEngine(&list, &rejected)
	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Test-IP", "203.0.113.9")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w.Code
	}
	if got := get(); got != http.StatusOK {
		t.Fatalf("before: %d", got)
	}
	list = "10.0.0.0/8"
	InvalidateIPAllowlist()
	if got := get(); got != http.StatusNotFound {
		t.Fatalf("after save: %d, want the new list applied", got)
	}
}

func TestIPAllowlistContains(t *testing.T) {
	if !IPAllowlistContains("", "203.0.113.9") {
		t.Fatal("an empty list admits everyone")
	}
	if IPAllowlistContains("10.0.0.0/8", "203.0.113.9") {
		t.Fatal("203.0.113.9 is not in 10.0.0.0/8")
	}
}
