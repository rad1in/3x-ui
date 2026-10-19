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

func newSalesEngine(t *testing.T) *gin.Engine {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	g := engine.Group("/panel/api/sales")
	g.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
	})
	NewSalesController(g)
	return engine
}

func postJSON(t *testing.T, engine *gin.Engine, path, body string) entity.Msg {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	return msg
}

func TestRefusedSalesSavesReadAsFailuresNotAsSaved(t *testing.T) {
	engine := newSalesEngine(t)
	cases := []struct {
		name, path, body, reason string
	}{
		{"config", "/panel/api/sales/config/save", `{"zarinpal":{"enabled":true}}`, "public base URL"},
		{"plan", "/panel/api/sales/plans/save", `{"name":"","currency":"USD","templateId":1}`, "plan name is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := postJSON(t, engine, tc.path, tc.body)
			if msg.Success || strings.Contains(msg.Msg, "saved") || !strings.Contains(msg.Msg, tc.reason) {
				t.Fatalf("msg = %+v; a refused save must not read as saved and must give its reason %q", msg, tc.reason)
			}
		})
	}
}

func TestSalesConfigIsServedWithoutItsSecrets(t *testing.T) {
	engine := newSalesEngine(t)
	saved := postJSON(t, engine, "/panel/api/sales/config/save",
		`{"publicBaseUrl":"https://s.example.com","zarinpal":{"enabled":true,"merchantId":"SECRET-MERCHANT"},"idpay":{"apiKey":"SECRET-KEY"}}`)
	if !saved.Success {
		t.Fatalf("save: %+v", saved)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/sales/config", nil))
	body := w.Body.String()
	if strings.Contains(body, "SECRET-MERCHANT") || strings.Contains(body, "SECRET-KEY") {
		t.Fatalf("config response leaks a secret: %s", body)
	}
	if !strings.Contains(body, `"hasZarinpalMerchantId":true`) || !strings.Contains(body, `"hasIdpayApiKey":true`) {
		t.Fatalf("config response = %s, want the Has flags set", body)
	}
}
