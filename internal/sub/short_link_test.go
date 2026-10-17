package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestShortLinkServesTheSubscription(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "long-sub-id", "short", 4482, 1, `{"network":"tcp","security":"none"}`)
	gin.SetMode(gin.TestMode)
	var links service.SubShortLinkService
	link, err := links.Ensure("long-sub-id")
	if err != nil {
		t.Fatal(err)
	}
	get := func(router *gin.Engine, path string) *httptest.ResponseRecorder {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "http://sub.example.com"+path, nil))
		return resp
	}
	newRouter := func(enabled bool) *gin.Engine {
		router := gin.New()
		NewSUBController(router.Group("/"), WithSUBShortLinks(enabled))
		return router
	}

	router := newRouter(true)
	long := get(router, "/sub/long-sub-id")
	short := get(router, "/s/"+link.Token)
	if short.Code != http.StatusOK || short.Body.String() != long.Body.String() || long.Body.Len() == 0 {
		t.Fatalf("short link = %d %q, want the long link's body %q", short.Code, short.Body.String(), long.Body.String())
	}
	if got := get(router, "/s/NoSuchTok1"); got.Code != http.StatusNotFound {
		t.Fatalf("unknown token = %d, want 404", got.Code)
	}

	if err := links.Revoke("long-sub-id"); err != nil {
		t.Fatal(err)
	}
	if got := get(router, "/s/"+link.Token); got.Code != http.StatusNotFound {
		t.Fatalf("revoked token = %d, want 404", got.Code)
	}

	renewed, _ := links.Ensure("long-sub-id")
	if got := get(newRouter(false), "/s/"+renewed.Token); got.Code != http.StatusNotFound {
		t.Fatalf("short links off: /s/ answered %d, want 404", got.Code)
	}
}
