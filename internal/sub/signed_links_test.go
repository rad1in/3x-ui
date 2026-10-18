package sub

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/util/subsign"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestSignedSubscriptionLinks(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "signed-1", "sg1", 4491, 1, `{"network":"tcp","security":"none"}`)
	seedSubInbound(t, "signed-2", "sg2", 4492, 1, `{"network":"tcp","security":"none"}`)
	gin.SetMode(gin.TestMode)
	get := func(router *gin.Engine, path string) int {
		req := httptest.NewRequest(http.MethodGet, "http://sub.example.com"+path, nil)
		req.RemoteAddr = "198.51.100.7:1000"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp.Code
	}
	newRouter := func() *gin.Engine {
		router := gin.New()
		NewSUBController(router.Group("/"), WithSUBJsonEnabled(true))
		return router
	}

	if code := get(newRouter(), "/sub/signed-1"); code != http.StatusOK {
		t.Fatalf("an unsigned link with signing off = %d, want 200", code)
	}

	cfgSvc := &service.SecurityConfigService{}
	if err := cfgSvc.Save(&service.SecurityConfig{SubSigningEnable: true}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := cfgSvc.Get()
	key := []byte(cfg.SubSigningKey)
	query := func(subID string, exp int64) string {
		return "?exp=" + strconv.FormatInt(exp, 10) + "&sig=" + subsign.Sign(key, subID, exp)
	}
	router := newRouter()
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	for name, tc := range map[string]struct {
		path string
		want int
	}{
		"unsigned":                   {"/sub/signed-1", http.StatusNotFound},
		"signed raw":                 {"/sub/signed-1" + query("signed-1", future), http.StatusOK},
		"signed without expiry":      {"/sub/signed-1" + query("signed-1", 0), http.StatusOK},
		"same signature for json":    {"/json/signed-1" + query("signed-1", future), http.StatusOK},
		"signature of another sub":   {"/sub/signed-1" + query("signed-2", future), http.StatusNotFound},
		"expired":                    {"/sub/signed-1" + query("signed-1", past), http.StatusNotFound},
		"forged signature":           {"/sub/signed-1?exp=0&sig=AAAA", http.StatusNotFound},
		"signature without expiry":   {"/sub/signed-1?sig=" + subsign.Sign(key, "signed-1", 0), http.StatusOK},
		"expiry without a signature": {"/sub/signed-1?exp=" + strconv.FormatInt(future, 10), http.StatusNotFound},
	} {
		if got := get(router, tc.path); got != tc.want {
			t.Errorf("%s: GET %s = %d, want %d", name, tc.path, got, tc.want)
		}
	}
}
