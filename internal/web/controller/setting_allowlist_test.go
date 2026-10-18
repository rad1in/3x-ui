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
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// A save whose list leaves out the saver's own address would end their session
// at the next request, with no way back but the server shell.
func TestUpdateSettingRefusesAnAllowlistThatLocksTheSaverOut(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	settings := service.SettingService{}

	save := func(list string) map[string]any {
		t.Helper()
		base, err := settings.GetAllSetting()
		if err != nil {
			t.Fatalf("GetAllSetting: %v", err)
		}
		raw, _ := json.Marshal(base)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		body["panelAllowedCidrs"] = list
		payload, _ := json.Marshal(body)

		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		})
		NewSettingController(router.Group("/panel/api"))
		req := httptest.NewRequest(http.MethodPost, "/panel/api/setting/update", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.10:5555"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		out := map[string]any{}
		_ = json.Unmarshal(resp.Body.Bytes(), &out)
		return out
	}
	stored := func() string {
		v, _ := settings.GetPanelAllowedCidrs()
		return v
	}

	if out := save("10.0.0.0/8"); out["success"] == true {
		t.Fatalf("a list without the saver's IP was accepted: %v", out)
	}
	if got := stored(); got != "" {
		t.Fatalf("rejected list was stored anyway: %q", got)
	}
	if out := save("10.0.0.0/8, 192.0.2.0/24"); out["success"] != true {
		t.Fatalf("a list holding the saver's IP was refused: %v", out)
	}
	if got := stored(); got != "10.0.0.0/8, 192.0.2.0/24" {
		t.Fatalf("stored list = %q", got)
	}
	if out := save(""); out["success"] != true || stored() != "" {
		t.Fatalf("clearing the list must always work: %v / %q", out, stored())
	}
}
