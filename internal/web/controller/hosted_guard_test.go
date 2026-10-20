package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

func hostedGuardEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/panel/api")
	api.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
	})
	api.Use(hostedGuard)
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	api.POST("/server/importDB", ok)
	api.POST("/server/installXray/:version", ok)
	api.POST("/clients/add", ok)
	api.POST("/sales/plans/save", ok)
	api.POST("/sales/orders/approve/:id", ok)
	return engine
}

func TestHostedGuardBlocksEscapesOnlyOnHostedPanels(t *testing.T) {
	cases := []struct {
		path       string
		hostedWant int
	}{
		{"/panel/api/server/importDB", http.StatusForbidden},
		{"/panel/api/server/installXray/v26.1.1", http.StatusForbidden},
		{"/panel/api/clients/add", http.StatusNoContent},
		{"/panel/api/sales/plans/save", http.StatusForbidden},
		{"/panel/api/sales/orders/approve/3", http.StatusForbidden},
	}
	for _, tc := range cases {
		for _, isHosted := range []bool{false, true} {
			dir := ""
			want := http.StatusNoContent
			if isHosted {
				dir, want = t.TempDir(), tc.hostedWant
			}
			t.Setenv(hosted.EnvDir, dir)
			w := httptest.NewRecorder()
			hostedGuardEngine().ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, nil))
			if w.Code != want {
				t.Errorf("hosted=%v %s: status %d, want %d", isHosted, tc.path, w.Code, want)
			}
		}
	}
}
