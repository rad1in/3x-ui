package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func mayCall(t *testing.T, role, method, path string) bool {
	t.Helper()
	var allowed, ran bool
	r := gin.New()
	r.Handle(method, "/panel/api"+path, func(c *gin.Context) {
		ran = true
		var err error
		allowed, err = adminMayCall(c, &model.User{Role: role, ScopeInboundIds: "[]", ScopeGroups: "[]"})
		if err != nil {
			t.Errorf("adminMayCall: %v", err)
		}
	})
	req := httptest.NewRequest(method, "/panel/api"+path, strings.NewReader("{}"))
	r.ServeHTTP(httptest.NewRecorder(), req)
	if !ran {
		t.Fatalf("%s %s never reached the handler", method, path)
	}
	return allowed
}

// A reseller must never move clients between resellers, itself included.
func TestOnlyOwnersAndAdminsMayTransferClients(t *testing.T) {
	for role, want := range map[string]bool{
		model.AdminRoleOwner:    true,
		model.AdminRoleAdmin:    true,
		model.AdminRoleOperator: false,
	} {
		if got := mayCall(t, role, http.MethodPost, "/clients/transfer"); got != want {
			t.Errorf("%s may call POST /clients/transfer = %v, want %v", role, got, want)
		}
	}
}

// A reseller can read its own sales but never another reseller's credit or the price list.
func TestResellerRoutesKeepCreditAndPricesOwnerOnly(t *testing.T) {
	cases := []struct {
		method, path string
		role         string
		want         bool
	}{
		{http.MethodGet, "/reseller/me", model.AdminRoleOperator, true},
		{http.MethodGet, "/reseller/report", model.AdminRoleOperator, true},
		{http.MethodPost, "/reseller/topup/:id", model.AdminRoleOperator, false},
		{http.MethodPost, "/reseller/prices/:id", model.AdminRoleOperator, false},
		{http.MethodGet, "/reseller/account/:id", model.AdminRoleOperator, false},
		{http.MethodGet, "/reseller/config", model.AdminRoleOperator, false},
		{http.MethodPost, "/reseller/topup/:id", model.AdminRoleAdmin, false},
		{http.MethodPost, "/reseller/topup/:id", model.AdminRoleOwner, true},
	}
	for _, c := range cases {
		if got := mayCall(t, c.role, c.method, c.path); got != c.want {
			t.Errorf("%s may call %s %s = %v, want %v", c.role, c.method, c.path, got, c.want)
		}
	}
}
