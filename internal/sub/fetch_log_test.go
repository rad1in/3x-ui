package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestSubscriptionFetchesAreLoggedWithAppIPAndFormat(t *testing.T) {
	seedSubDB(t)
	seedSingboxSub(t, "s1")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBClashEnabled(true), WithSUBJsonEnabled(true))
	fetch := func(path, remote, ua string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://sub.example.com"+path, nil)
		req.RemoteAddr = remote
		req.Header.Set("User-Agent", ua)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, resp.Code)
		}
	}
	fetch("/sub/s1", "198.51.100.7:5000", "v2rayNG/1.10.2")
	fetch("/sub/s1", "198.51.100.7:5001", "v2rayNG/1.10.2")
	fetch("/clash/s1", "203.0.113.9:6000", "Clash-Verge/2.0")
	fetch("/json/s1", "203.0.113.9:6001", "Hiddify/2.5")
	fetch("/singbox/s1", "203.0.113.9:6002", "sing-box/1.12")
	fetch("/loon/s1", "203.0.113.9:6003", "Loon/1")

	got, err := (&service.SubFetchService{}).List("s1")
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ ip, ua, format string }
	counts := map[key]int{}
	for _, e := range got {
		counts[key{e.IP, e.UserAgent, e.Format}] = e.Count
	}
	want := map[key]int{
		{"198.51.100.7", "v2rayNG/1.10.2", "raw"}:   2,
		{"203.0.113.9", "Clash-Verge/2.0", "clash"}: 1,
		{"203.0.113.9", "Hiddify/2.5", "json"}:      1,
		{"203.0.113.9", "sing-box/1.12", "singbox"}: 1,
		{"203.0.113.9", "Loon/1", "loon"}:           1,
	}
	if len(counts) != len(want) {
		t.Fatalf("log = %+v, want %+v", counts, want)
	}
	for k, n := range want {
		if counts[k] != n {
			t.Fatalf("log = %+v, want %+v", counts, want)
		}
	}
}
