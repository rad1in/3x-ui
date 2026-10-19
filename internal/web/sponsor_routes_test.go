package web

import (
	"net/http"
	"strings"
	"testing"
)

// Sponsors come only from the owner's remote file: no route may write them or
// serve a local feed or admin list, so a panel admin or reseller has nothing to abuse.
func TestNoSponsorWriteOrFeedRouteExists(t *testing.T) {
	routes := registeredContractRoutes(t)
	for route := range routes {
		method, path, _ := strings.Cut(route, " ")
		if !strings.Contains(strings.ToLower(path), "sponsor") {
			continue
		}
		if method != http.MethodGet {
			t.Errorf("sponsor route %s must not exist: sponsors are read-only", route)
		}
		if strings.Contains(path, "/feed") || strings.HasPrefix(path, "/panel/api/sponsors") {
			t.Errorf("sponsor route %s must not exist: sponsors have no panel-side API or feed", route)
		}
	}
}
