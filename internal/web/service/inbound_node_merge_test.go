package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// MySQL evaluates UPDATE ... SET left to right against already-updated
// columns, so the enable verdict must not see this merge's delta twice.
func TestMergeNodeClientTrafficJudgesStaleDisableOnPreMergeCounters(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	master := xray.ClientTraffic{InboundId: 1, Email: "quota@x", Enable: true, Total: 1000, Up: 400}
	if err := db.Create(&master).Error; err != nil {
		t.Fatalf("seed client traffic: %v", err)
	}

	// The node disabled the client under an old 500-byte quota; the master
	// has since raised it to 1000, and 400 + 300 is still under that.
	node := xray.ClientTraffic{Email: "quota@x", Enable: false, Total: 500, LastOnline: 1}
	s := &InboundService{}
	if err := s.mergeNodeClientTraffic(db, node, false, 300, 0, time.Now().UnixMilli()); err != nil {
		t.Fatalf("mergeNodeClientTraffic: %v", err)
	}

	var got xray.ClientTraffic
	if err := db.Where("email = ?", "quota@x").First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Up != 700 || !got.Enable {
		t.Fatalf("after merge up=%d enable=%v, want up=700 enable=true", got.Up, got.Enable)
	}
}
