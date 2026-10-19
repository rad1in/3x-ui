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

// An operator must never move clients between admins, itself included.
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
