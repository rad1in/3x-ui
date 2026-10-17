package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// xraylessLocal is the panel's own runtime with no Xray to talk to: a per-user
// Xray op fails as it would there, and every sidecar re-apply is counted.
type xraylessLocal struct {
	fakeNodeRuntime
	xrayOps atomic.Int32
}

func (x *xraylessLocal) AddUser(context.Context, *model.Inbound, map[string]any) error {
	x.xrayOps.Add(1)
	return errors.New("local xray is not running")
}

func (x *xraylessLocal) RemoveUser(context.Context, *model.Inbound, string) error {
	x.xrayOps.Add(1)
	return errors.New("local xray is not running")
}

// seedSidecarClients is an OpenVPN inbound, served outside Xray, with clients.
func seedSidecarClients(t *testing.T, emails ...string) (*InboundService, *ClientService, int, *xraylessLocal) {
	t.Helper()
	setupConflictDB(t)
	startSerializedWriter(t)
	local := &xraylessLocal{}
	useTestRuntimeManager(t).SetLocalRuntimeOverride(local)
	inbounds := &InboundService{}
	ib := &model.Inbound{Protocol: model.OpenVPN, Settings: `{}`}
	if err := inbounds.normalizeOpenVPNSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ovpn-bulk", "", 1194, model.OpenVPN, "", ib.Settings)
	id := loadInboundByTag(t, "ovpn-bulk").Id
	clients := &ClientService{}
	for _, email := range emails {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: model.Client{Email: email, Enable: true}, InboundIds: []int{id}}); err != nil {
			t.Fatal(err)
		}
	}
	local.updateInbound.Store(0)
	local.xrayOps.Store(0)
	return inbounds, clients, id, local
}

// A sidecar takes a client change by re-applying the inbound to its manager,
// which drops the revoked session itself; Xray per-user ops only fail there
// and turn into a needless Xray restart.
func assertSidecarApplied(t *testing.T, op string, local *xraylessLocal, needRestart bool, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	if needRestart || local.xrayOps.Load() != 0 {
		t.Fatalf("%s sent %d Xray user ops (needRestart %v) to an OpenVPN inbound", op, local.xrayOps.Load(), needRestart)
	}
	if local.updateInbound.Load() == 0 {
		t.Fatalf("%s never re-applied the OpenVPN inbound, so the change waits for the next job tick", op)
	}
}

func TestBulkSetEnableReappliesASidecarInbound(t *testing.T) {
	inbounds, clients, _, local := seedSidecarClients(t, "bulk-a", "bulk-b")
	_, needRestart, err := clients.BulkSetEnable(inbounds, []string{"bulk-a"}, false)
	assertSidecarApplied(t, "BulkSetEnable", local, needRestart, err)
}

func TestBulkDeleteReappliesASidecarInbound(t *testing.T) {
	inbounds, clients, _, local := seedSidecarClients(t, "del-a", "del-b")
	_, needRestart, err := clients.BulkDelete(inbounds, []string{"del-a"}, false)
	assertSidecarApplied(t, "BulkDelete", local, needRestart, err)
}

func TestBulkDetachReappliesASidecarInbound(t *testing.T) {
	inbounds, clients, id, local := seedSidecarClients(t, "det-a", "det-b")
	_, needRestart, err := clients.BulkDetach(inbounds, []string{"det-a"}, []int{id})
	assertSidecarApplied(t, "BulkDetach", local, needRestart, err)
}

// A traffic reset re-enables a depleted client, which a sidecar must serve again.
func TestResetTrafficReappliesASidecarInbound(t *testing.T) {
	inbounds, _, id, local := seedSidecarClients(t, "reset-a")
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "reset-a").
		Updates(map[string]any{"enable": false, "up": 1 << 30}).Error; err != nil {
		t.Fatal(err)
	}
	needRestart, err := inbounds.ResetClientTraffic(id, "reset-a")
	assertSidecarApplied(t, "ResetClientTraffic", local, needRestart, err)
}
