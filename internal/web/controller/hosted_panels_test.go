package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

func TestHostedPanelSaveFailureIsNotReportedAsSaved(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	g := engine.Group("/panel/api/hostedPanels")
	g.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
	})
	NewHostedPanelController(g)

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"","username":"u","password":"long-enough","webPort":20530,"subPort":20531,"portMin":30000,"portMax":30099}`)
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/panel/api/hostedPanels/add", body))

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Success || !strings.HasPrefix(msg.Msg, "fail (") || !strings.Contains(msg.Msg, "name is required") {
		t.Fatalf("msg = %+v; a refused save must read as a failure with its reason", msg)
	}
}
