package web

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/controller"
)

// A read-only POST missing from the skip list floods the activity log; a stale
// skip entry hides nothing today but would silently hide a route reusing the path.
func TestAuditRouteClassification(t *testing.T) {
	registered := registeredContractRoutes(t)

	for _, skipped := range controller.AuditSkippedRoutes() {
		method, rel, _ := strings.Cut(skipped, " ")
		if !registered[method+" /panel/api"+rel] {
			t.Errorf("audit skip entry %q matches no registered route", skipped)
		}
	}

	for _, route := range []string{
		"POST /panel/api/clients/add",
		"POST /panel/api/inbounds/update/:id",
		"POST /panel/api/setting/update",
		"POST /panel/api/setting/updateUser",
		"DELETE /panel/api/clients/hwids/:email",
	} {
		method, path, _ := strings.Cut(route, " ")
		if !registered[route] {
			t.Fatalf("expected route %s is no longer registered; update this test", route)
		}
		if !controller.IsAuditedRoute(method, path) {
			t.Errorf("mutation %s is not audited", route)
		}
	}
}

// A renamed route would silently drop out of the role tables: an owner-only
// route would open to admins, an operator route would close for operators.
func TestRBACRoutesExist(t *testing.T) {
	registered := registeredContractRoutes(t)
	for _, route := range controller.RBACRoutes() {
		method, rel, _ := strings.Cut(route, " ")
		if !registered[method+" /panel/api"+rel] {
			t.Errorf("role table entry %q matches no registered route", route)
		}
	}
}

// A renamed route would silently drop out of the hosted block list and let a
// hosted panel escape its limits.
func TestHostedForbiddenRoutesExist(t *testing.T) {
	registered := registeredContractRoutes(t)
	for _, route := range controller.HostedForbiddenRoutes() {
		method, rel, _ := strings.Cut(route, " ")
		if !registered[method+" /panel/api"+rel] {
			t.Errorf("hosted block entry %q matches no registered route", route)
		}
	}
}
