package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Only inbounds that should be serving someone are reported: an OpenVPN
// inbound without a live client is idle by design, not broken.
func TestVPNStatusesCoverInboundsExpectedToRun(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	for tag, clients := range map[string]string{
		"ovpn-idle": `[]`,
		"ovpn-used": `[{"email":"u1","password":"p1","enable":true}]`,
	} {
		ib := &model.Inbound{Protocol: model.OpenVPN, Settings: `{"clients":` + clients + `}`}
		if err := svc.normalizeOpenVPNSettings(ib, ""); err != nil {
			t.Fatal(err)
		}
		seedInboundConflict(t, tag, "", 1194+len(tag), model.OpenVPN, "", ib.Settings)
	}
	used := loadInboundByTag(t, "ovpn-used").Id
	seedClientTraffic(t, used, "u1", true)

	statuses, err := svc.VPNStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Id != used || statuses[0].Running {
		t.Fatalf("statuses = %+v; want only the used inbound, not yet started", statuses)
	}
}
