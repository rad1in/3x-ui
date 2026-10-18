package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

// A stolen token must be useless from an address the owner never listed, while
// tokens without a list keep working from anywhere.
func TestAPITokenAllowedCidrsGateTheCaller(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	seed := func(name, plaintext, cidrs string) {
		t.Helper()
		err := database.GetDB().Create(&model.ApiToken{
			Name: name, Token: crypto.HashTokenSHA256(plaintext), Enabled: true,
			Scope: model.ApiScopeAdmin, AllowedCidrs: cidrs,
		}).Error
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	seed("pinned", "pinned-token-value", "198.51.100.0/24, 203.0.113.9")
	seed("open", "open-token-value", "")

	cases := []struct {
		name, token, from string
		want              int
	}{
		{"inside the listed network", "pinned-token-value", "198.51.100.77:4000", http.StatusOK},
		{"the listed single address", "pinned-token-value", "203.0.113.9:4000", http.StatusOK},
		{"outside the list", "pinned-token-value", "192.0.2.1:4000", http.StatusUnauthorized},
		{"unlisted token works anywhere", "open-token-value", "192.0.2.1:4000", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/panel/api/ping", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.RemoteAddr = tc.from
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
