package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestCapPeriodStart(t *testing.T) {
	loc := time.UTC
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, loc) }
	cases := []struct {
		name string
		now  time.Time
		day  int
		want time.Time
	}{
		{"day one, mid month", at(2026, 3, 10, 12), 1, at(2026, 3, 1, 0)},
		{"before the billing day rolls back a month", at(2026, 3, 10, 12), 15, at(2026, 2, 15, 0)},
		{"on the billing day it has started", at(2026, 3, 15, 0), 15, at(2026, 3, 15, 0)},
		{"rolls back across the year", at(2026, 1, 5, 12), 20, at(2025, 12, 20, 0)},
		{"day above 28 is clamped to 28", at(2026, 2, 27, 12), 31, at(2026, 1, 28, 0)},
		{"day zero means day one", at(2026, 3, 10, 12), 0, at(2026, 3, 1, 0)},
	}
	for _, tc := range cases {
		if got := capPeriodStart(tc.now, tc.day); !got.Equal(tc.want) {
			t.Errorf("%s: capPeriodStart(%v, %d) = %v, want %v", tc.name, tc.now, tc.day, got, tc.want)
		}
	}
}

const gib = int64(1) << 30

func seedCapNode(t *testing.T, capGB float64, action string) (*model.Node, *model.Inbound) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	node := &model.Node{
		Name: "metered", Address: "n1", Port: 2053, Enable: true, Status: "offline",
		MonthlyCapGB: capGB, CapAction: action, BillingDay: 1,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := node.Id
	ib := &model.Inbound{Tag: "in-1", Port: 443, Protocol: model.VLESS, Enable: true, NodeID: &nodeID, Settings: `{"clients":[]}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	return node, ib
}

func setInboundTraffic(t *testing.T, id int, down int64) {
	t.Helper()
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", id).Update("down", down).Error; err != nil {
		t.Fatalf("set traffic: %v", err)
	}
}

func reloadNode(t *testing.T, id int) model.Node {
	t.Helper()
	var n model.Node
	if err := database.GetDB().First(&n, id).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	return n
}

func TestNodeCapTickCountsDeltasAndFiresOncePerPeriod(t *testing.T) {
	node, ib := seedCapNode(t, 1, "alert")
	s := &NodeCapService{}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	setInboundTraffic(t, ib.Id, 5*gib) // history from before the cap was set must not count
	if hits := s.Tick(now); len(hits) != 0 {
		t.Fatalf("priming tick fired %v", hits)
	}
	if n := reloadNode(t, node.Id); n.PeriodBytes != 0 {
		t.Fatalf("priming counted %d bytes of history", n.PeriodBytes)
	}

	setInboundTraffic(t, ib.Id, 5*gib+600<<20)
	if hits := s.Tick(now.Add(time.Minute)); len(hits) != 0 {
		t.Fatalf("under the cap fired %v", hits)
	}
	if n := reloadNode(t, node.Id); n.PeriodBytes != 600<<20 {
		t.Fatalf("PeriodBytes = %d, want %d", n.PeriodBytes, int64(600<<20))
	}

	setInboundTraffic(t, ib.Id, 6*gib+200<<20)
	hits := s.Tick(now.Add(2 * time.Minute))
	if len(hits) != 1 || hits[0].NodeId != node.Id || hits[0].Action != "alert" || hits[0].CapBytes != gib {
		t.Fatalf("crossing the cap gave %+v, want one alert for node %d capped at %d", hits, node.Id, gib)
	}
	if n := reloadNode(t, node.Id); n.CapState != capStateReached {
		t.Fatalf("CapState = %q, want %q", n.CapState, capStateReached)
	}

	setInboundTraffic(t, ib.Id, 6*gib+300<<20)
	if hits := s.Tick(now.Add(3 * time.Minute)); len(hits) != 0 {
		t.Fatalf("a reached cap fired again: %v", hits)
	}

	// An inbound reset or delete lowers the sum; that must neither refund nor over-count.
	before := reloadNode(t, node.Id).PeriodBytes
	setInboundTraffic(t, ib.Id, 0)
	s.Tick(now.Add(4 * time.Minute))
	if n := reloadNode(t, node.Id); n.PeriodBytes != before {
		t.Fatalf("PeriodBytes moved from %d to %d across a counter reset", before, n.PeriodBytes)
	}
}

func TestNodeCapDisableActionStopsInboundsAndRolloverRestoresOnlyThose(t *testing.T) {
	node, ib := seedCapNode(t, 1, "disable")
	db := database.GetDB()
	nodeID := node.Id
	adminOff := &model.Inbound{Tag: "in-off", Port: 8443, Protocol: model.VLESS, Enable: false, NodeID: &nodeID, Settings: `{"clients":[]}`}
	if err := db.Create(adminOff).Error; err != nil {
		t.Fatalf("seed disabled inbound: %v", err)
	}
	s := &NodeCapService{}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	s.Tick(now)
	setInboundTraffic(t, ib.Id, 2*gib)
	hits := s.Tick(now.Add(time.Minute))
	if len(hits) != 1 || hits[0].Action != "disable" {
		t.Fatalf("hits = %+v, want one disable", hits)
	}
	var got model.Inbound
	if err := db.First(&got, ib.Id).Error; err != nil || got.Enable {
		t.Fatalf("capped inbound enable=%v err=%v, want disabled", got.Enable, err)
	}

	// The next billing period starts: the cap resets and only our inbound returns.
	s.Tick(time.Date(2026, 4, 1, 0, 1, 0, 0, time.UTC))
	if err := db.First(&got, ib.Id).Error; err != nil || !got.Enable {
		t.Fatalf("capped inbound enable=%v err=%v after rollover, want re-enabled", got.Enable, err)
	}
	var off model.Inbound
	if err := db.First(&off, adminOff.Id).Error; err != nil || off.Enable {
		t.Fatalf("admin-disabled inbound enable=%v err=%v, want it left disabled", off.Enable, err)
	}
	n := reloadNode(t, node.Id)
	if n.PeriodBytes != 0 || n.CapState != "" || len(n.CapDisabledInbounds) != 0 {
		t.Fatalf("after rollover bytes=%d state=%q disabled=%v, want a clean period", n.PeriodBytes, n.CapState, n.CapDisabledInbounds)
	}
}

func TestNodeCapTickIgnoresNodesWithoutACap(t *testing.T) {
	node, ib := seedCapNode(t, 0, "")
	s := &NodeCapService{}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	s.Tick(now)
	setInboundTraffic(t, ib.Id, 900*gib)
	if hits := s.Tick(now.Add(time.Minute)); len(hits) != 0 {
		t.Fatalf("uncapped node fired %v", hits)
	}
	if n := reloadNode(t, node.Id); n.PeriodBytes != 900*gib {
		t.Fatalf("usage still tracked for cost view: PeriodBytes = %d, want %d", n.PeriodBytes, 900*gib)
	}
}

func TestNodeCapRaisingTheCapMidPeriodRestoresStoppedInbounds(t *testing.T) {
	node, ib := seedCapNode(t, 1, "disable")
	s := &NodeCapService{}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	s.Tick(now)
	setInboundTraffic(t, ib.Id, 2*gib)
	if hits := s.Tick(now.Add(time.Minute)); len(hits) != 1 {
		t.Fatalf("hits = %+v, want the cap reached", hits)
	}

	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", node.Id).Update("monthly_cap_gb", 10).Error; err != nil {
		t.Fatalf("raise cap: %v", err)
	}
	if hits := s.Tick(now.Add(2 * time.Minute)); len(hits) != 0 {
		t.Fatalf("raised cap fired %v", hits)
	}
	var got model.Inbound
	if err := database.GetDB().First(&got, ib.Id).Error; err != nil || !got.Enable {
		t.Fatalf("inbound enable=%v err=%v after the cap was raised, want re-enabled", got.Enable, err)
	}
	if n := reloadNode(t, node.Id); n.CapState != "" {
		t.Fatalf("CapState = %q, want cleared", n.CapState)
	}
}
