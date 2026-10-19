package sub

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type failoverSetup struct {
	primary, backup *model.Node
	onPrimary       *model.Inbound
}

func seedFailover(t *testing.T, backupPort int, backupEnabled bool) failoverSetup {
	t.Helper()
	initSubDB(t)
	db := database.GetDB()
	backup := &model.Node{Name: "backup", Address: "backup.example.com", Port: 2053, Enable: true, Status: "online"}
	if err := db.Create(backup).Error; err != nil {
		t.Fatal(err)
	}
	primary := &model.Node{
		Name: "primary", Address: "primary.example.com", Port: 2053, Enable: true, Status: "offline",
		LastHeartbeat: time.Now().Add(-10 * time.Minute).Unix(), FailoverNodeId: backup.Id, FailoverAfterSec: 120,
	}
	if err := db.Create(primary).Error; err != nil {
		t.Fatal(err)
	}
	onPrimary := &model.Inbound{Tag: "p-443", Port: 443, Protocol: model.VLESS, Enable: true, NodeID: &primary.Id, Settings: `{"clients":[]}`}
	onBackup := &model.Inbound{Tag: "b-443", Port: backupPort, Protocol: model.VLESS, Enable: true, NodeID: &backup.Id, Settings: `{"clients":[]}`}
	for _, ib := range []*model.Inbound{onPrimary, onBackup} {
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
	}
	if !backupEnabled {
		db.Model(onBackup).Update("enable", false)
	}
	return failoverSetup{primary: primary, backup: backup, onPrimary: onPrimary}
}

func addressFor(t *testing.T, ib *model.Inbound) string {
	t.Helper()
	s := &SubService{}
	s.loadNodes()
	return s.resolveInboundAddress(ib)
}

func TestSubscriptionAdvertisesTheBackupNodeWhileTheNodeIsDown(t *testing.T) {
	f := seedFailover(t, 443, true)
	if got := addressFor(t, f.onPrimary); got != "backup.example.com" {
		t.Fatalf("address = %q, want the backup node's", got)
	}
}

func TestSubscriptionKeepsTheNodeAddressWhenItIsBackOrFailoverCannotServe(t *testing.T) {
	t.Run("node is back", func(t *testing.T) {
		f := seedFailover(t, 443, true)
		database.GetDB().Model(f.primary).Update("status", "online")
		if got := addressFor(t, f.onPrimary); got != "primary.example.com" {
			t.Fatalf("address = %q after recovery", got)
		}
	})
	t.Run("backup has no inbound on the same port", func(t *testing.T) {
		f := seedFailover(t, 8443, true)
		if got := addressFor(t, f.onPrimary); got != "primary.example.com" {
			t.Fatalf("address = %q, the backup cannot serve port 443", got)
		}
	})
	t.Run("backup inbound is disabled", func(t *testing.T) {
		f := seedFailover(t, 443, false)
		if got := addressFor(t, f.onPrimary); got != "primary.example.com" {
			t.Fatalf("address = %q, the backup inbound is off", got)
		}
	})
	t.Run("grace period not over", func(t *testing.T) {
		f := seedFailover(t, 443, true)
		database.GetDB().Model(f.primary).Update("last_heartbeat", time.Now().Add(-10*time.Second).Unix())
		if got := addressFor(t, f.onPrimary); got != "primary.example.com" {
			t.Fatalf("address = %q inside the grace period", got)
		}
	})
}

func TestFailoverDoesNotOverrideAnExplicitShareAddress(t *testing.T) {
	f := seedFailover(t, 443, true)
	f.onPrimary.ShareAddrStrategy = "custom"
	f.onPrimary.ShareAddr = "vpn.example.com"
	if got := addressFor(t, f.onPrimary); got != "vpn.example.com" {
		t.Fatalf("address = %q, an admin-chosen share address must stay", got)
	}
}
