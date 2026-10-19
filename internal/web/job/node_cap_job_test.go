package job

import (
	"path/filepath"
	"testing"

	"github.com/op/go-logging"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
)

func TestNodeCapJobPublishesTheCapReachedEventOnce(t *testing.T) {
	xuilogger.InitLogger(logging.ERROR)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	db := database.GetDB()
	node := &model.Node{Name: "metered-1", Address: "n1", Port: 2053, Enable: true, Status: "offline", MonthlyCapGB: 1, CapAction: "alert", BillingDay: 1}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := node.Id
	ib := &model.Inbound{Tag: "in-1", Port: 443, Protocol: model.VLESS, Enable: true, NodeID: &nodeID, Settings: `{"clients":[]}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	events := collectNodeEvents(t)
	job := NewNodeCapJob()

	job.Run() // primes the counter baseline
	if err := db.Model(&model.Inbound{}).Where("id = ?", ib.Id).Update("down", int64(2<<30)).Error; err != nil {
		t.Fatalf("add traffic: %v", err)
	}
	job.Run()
	job.Run()

	got := events()
	if len(got) != 1 || got[0].Type != eventbus.EventNodeCapReached || got[0].Source != "metered-1" {
		t.Fatalf("events = %+v, want exactly one node.cap_reached for metered-1", got)
	}
	data, ok := got[0].Data.(*eventbus.NodeCapData)
	if !ok || data.NodeId != nodeID || data.CapBytes != 1<<30 || data.UsedBytes < 2<<30 || data.Stopped {
		t.Fatalf("event data = %+v", got[0].Data)
	}
}
