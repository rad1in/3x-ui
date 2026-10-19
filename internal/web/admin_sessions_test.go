package web

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func sessionIds(out map[string]any) map[string]float64 {
	ids := map[string]float64{}
	rows, _ := out["obj"].([]any)
	for _, r := range rows {
		m, _ := r.(map[string]any)
		ids[m["id"].(string)] = m["userId"].(float64)
	}
	return ids
}

// Everyone sees their own browsers, only an owner sees the rest, and ending a
// session really ends it.
func TestSessionsAreListedAndRevokedPerAdmin(t *testing.T) {
	f := newRBACFixture(t)
	owner := f.login(t, model.AdminRoleOwner)
	op := f.login(t, model.AdminRoleOperator)
	op2 := f.login(t, model.AdminRoleOperator)

	_, out := op.call(http.MethodGet, "/sessions/list", "")
	mine := sessionIds(out)
	if len(mine) != 2 {
		t.Fatalf("operator sees %d sessions, want its own two browsers: %v", len(mine), mine)
	}
	for _, uid := range mine {
		if int(uid) != f.userIds[model.AdminRoleOperator] {
			t.Fatalf("operator sees another admin's session: %v", mine)
		}
	}
	_, out = owner.call(http.MethodGet, "/sessions/list", "")
	all := sessionIds(out)
	if len(all) != 3 {
		t.Fatalf("owner sees %d sessions, want all three", len(all))
	}

	var ownerSession string
	for id, uid := range all {
		if int(uid) == f.userIds[model.AdminRoleOwner] {
			ownerSession = id
		}
	}
	if _, res := op.call(http.MethodPost, "/sessions/revoke/"+ownerSession, ""); res["success"] != false {
		t.Fatalf("operator ended the owner's session: %v", res)
	}
	if status, _ := owner.call(http.MethodGet, "/admins/me", ""); status != http.StatusOK {
		t.Fatal("the owner's session died after another admin's rejected revoke")
	}

	if _, res := op.call(http.MethodPost, "/sessions/revokeAll", `{"keepCurrent":true}`); res["success"] != true {
		t.Fatalf("revokeAll: %v", res)
	}
	if status, _ := op.call(http.MethodGet, "/admins/me", ""); status != http.StatusOK {
		t.Fatal("keepCurrent signed the caller out as well")
	}
	if status, _ := op2.call(http.MethodGet, "/admins/me", ""); status == http.StatusOK {
		t.Fatal("the operator's other browser survived log out everywhere")
	}

	body := fmt.Sprintf(`{"userId":%d}`, f.userIds[model.AdminRoleOperator])
	if _, res := owner.call(http.MethodPost, "/sessions/revokeAll", body); res["success"] != true {
		t.Fatalf("owner could not end another admin's sessions: %v", res)
	}
	if status, _ := op.call(http.MethodGet, "/admins/me", ""); status == http.StatusOK {
		t.Fatal("owner's revokeAll left the operator signed in")
	}
}
