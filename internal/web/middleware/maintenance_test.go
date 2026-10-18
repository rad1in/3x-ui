package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMaintenanceAnswers503WithTheNoticeAndLetsTrafficThroughWhenOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	on, message := true, "back at noon"
	router := gin.New()
	router.Use(MaintenanceMiddleware(func() (bool, string) { return on, message }))
	router.GET("/sub/abc", func(c *gin.Context) { c.String(http.StatusOK, "links") })

	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/abc", nil))
		return w
	}
	w := get()
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != "back at noon" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("during maintenance: %d %q retry-after=%q", w.Code, w.Body.String(), w.Header().Get("Retry-After"))
	}
	message = ""
	if w := get(); w.Code != http.StatusServiceUnavailable || w.Body.String() != DefaultMaintenanceMessage {
		t.Fatalf("with no custom message: %d %q", w.Code, w.Body.String())
	}
	on = false
	if w := get(); w.Code != http.StatusOK || w.Body.String() != "links" {
		t.Fatalf("after maintenance: %d %q", w.Code, w.Body.String())
	}
}
