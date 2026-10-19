package sub

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSubscriptionServesXrayJsonToSelectedApps(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "s1", "apps", 4481, 1, `{"network":"tcp","security":"none"}`)
	gin.SetMode(gin.TestMode)

	fetch := func(t *testing.T, apps, userAgent string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://sub.example.com/sub/s1", nil)
		req.Header.Set("User-Agent", userAgent)
		resp := httptest.NewRecorder()
		newSubscriptionTestRouter(subscriptionTestRouterConfig{jsonApps: apps}).ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
		}
		return resp
	}
	wantJSON := func(t *testing.T, resp *httptest.ResponseRecorder) {
		t.Helper()
		if got := resp.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q, want Xray JSON", got)
		}
		if body := resp.Body.String(); !strings.HasPrefix(strings.TrimSpace(body), "[") || !strings.Contains(body, `"outbounds"`) {
			t.Fatalf("body is not an Xray JSON array:\n%s", body)
		}
	}
	wantRaw := func(t *testing.T, resp *httptest.ResponseRecorder) {
		t.Helper()
		decoded, err := base64.StdEncoding.DecodeString(resp.Body.String())
		if err != nil || !strings.Contains(string(decoded), "vless://") {
			t.Fatalf("want the base64 link list, got %q (%v)", resp.Body.String(), err)
		}
	}

	t.Run("v2rayN selected gets JSON", func(t *testing.T) {
		wantJSON(t, fetch(t, "v2rayn", "v2rayN/7.10.0"))
	})
	t.Run("v2rayNG is not matched by the v2rayN entry", func(t *testing.T) {
		wantRaw(t, fetch(t, "v2rayn", "v2rayNG/1.10.2"))
	})
	t.Run("v2rayNG selected gets JSON", func(t *testing.T) {
		wantJSON(t, fetch(t, "v2rayng,streisand", "v2rayNG/1.10.2"))
	})
	t.Run("Streisand selected gets JSON", func(t *testing.T) {
		wantJSON(t, fetch(t, "v2rayng,streisand", "Streisand/2.1 CFNetwork/1568 Darwin/24.0"))
	})
	t.Run("unselected app keeps the raw list", func(t *testing.T) {
		wantRaw(t, fetch(t, "", "v2rayN/7.10.0"))
	})
}
