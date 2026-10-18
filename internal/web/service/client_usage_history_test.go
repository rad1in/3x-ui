package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func seedUsageClient(t *testing.T, port int, email string) {
	t.Helper()
	source := []model.Client{{Email: email, ID: "11111111-1111-1111-1111-111111111111", SubID: "s-" + email, Enable: true}}
	ib := mkInbound(t, port, model.VLESS, clientsSettings(t, source))
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, source); err != nil {
		t.Fatalf("seed linkage: %v", err)
	}
	mkTraffic(t, ib.Id, email, 0, 0, 0, 0, true)
}

func TestAddTrafficAccumulatesDailyUsage(t *testing.T) {
	setupBulkDB(t)
	seedUsageClient(t, 23001, "u@x")
	inboundSvc := &InboundService{}

	for _, delta := range [][2]int64{{100, 200}, {5, 7}} {
		if _, _, err := inboundSvc.AddTraffic(nil, []*xray.ClientTraffic{{Email: "u@x", Up: delta[0], Down: delta[1]}}); err != nil {
			t.Fatalf("AddTraffic: %v", err)
		}
	}
	// A tick with no bytes must not create a row for another client.
	if _, _, err := inboundSvc.AddTraffic(nil, []*xray.ClientTraffic{{Email: "u@x"}}); err != nil {
		t.Fatalf("AddTraffic idle: %v", err)
	}

	rows, err := (&ClientService{}).GetClientUsageHistory("u@x", 7)
	if err != nil {
		t.Fatalf("GetClientUsageHistory: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one row for today", rows)
	}
	if rows[0].Day != time.Now().Format("2006-01-02") || rows[0].Up != 105 || rows[0].Down != 207 {
		t.Fatalf("row = %+v, want today with up=105 down=207", rows[0])
	}
}

func TestGetClientUsageHistoryLimitsWindowAndOrdersByDay(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	for _, off := range []int{0, 3, 40} {
		day := now.AddDate(0, 0, -off).Format("2006-01-02")
		if err := database.GetDB().Create(&model.ClientDailyUsage{Email: "w@x", Day: day, Up: int64(off), Down: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, err := (&ClientService{}).GetClientUsageHistory("w@x", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Up != 3 || rows[1].Up != 0 {
		t.Fatalf("rows = %+v, want the 3-day-old row then today and nothing older than 7 days", rows)
	}
}

func TestPruneClientUsageHistoryDropsOnlyExpiredDays(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	old := now.Add(-ClientUsageRetention - 48*time.Hour).Format("2006-01-02")
	recent := now.Add(-ClientUsageRetention + 48*time.Hour).Format("2006-01-02")
	for _, day := range []string{old, recent} {
		if err := database.GetDB().Create(&model.ClientDailyUsage{Email: "p@x", Day: day, Up: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	removed, err := (&ClientService{}).PruneClientUsageHistory(now)
	if err != nil {
		t.Fatal(err)
	}
	var left []model.ClientDailyUsage
	database.GetDB().Find(&left)
	if removed != 1 || len(left) != 1 || left[0].Day != recent {
		t.Fatalf("removed=%d left=%+v, want only the old day pruned", removed, left)
	}
}

func TestClientUsageHistoryFollowsRenameAndDelete(t *testing.T) {
	setupBulkDB(t)
	day := time.Now().Format("2006-01-02")
	db := database.GetDB()
	if err := db.Create(&model.ClientDailyUsage{Email: "old@x", Day: day, Up: 9}).Error; err != nil {
		t.Fatal(err)
	}
	if err := renameClientUsage(db, "old@x", "new@x"); err != nil {
		t.Fatal(err)
	}
	rows, _ := (&ClientService{}).GetClientUsageHistory("new@x", 7)
	if len(rows) != 1 || rows[0].Up != 9 {
		t.Fatalf("rows after rename = %+v", rows)
	}
	if err := deleteClientUsage(db, []string{"new@x"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = (&ClientService{}).GetClientUsageHistory("new@x", 7)
	if len(rows) != 0 {
		t.Fatalf("rows after delete = %+v", rows)
	}
}
