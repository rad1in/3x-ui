package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeFailoverTarget(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	backup := &model.Node{Id: 2, Address: "backup.example.com", Enable: true, Status: "online"}
	primary := func(mod func(*model.Node)) *model.Node {
		n := &model.Node{Id: 1, Address: "a.example.com", Enable: true, Status: "offline",
			LastHeartbeat: now.Add(-5 * time.Minute).Unix(), FailoverNodeId: 2, FailoverAfterSec: 120}
		if mod != nil {
			mod(n)
		}
		return n
	}
	cases := []struct {
		name   string
		node   *model.Node
		others []*model.Node
		want   int
	}{
		{"down past the grace period", primary(nil), []*model.Node{backup}, 2},
		{"down but still inside the grace period", primary(func(n *model.Node) { n.LastHeartbeat = now.Add(-30 * time.Second).Unix() }), []*model.Node{backup}, 0},
		{"exactly at the grace period", primary(func(n *model.Node) { n.LastHeartbeat = now.Add(-120 * time.Second).Unix() }), []*model.Node{backup}, 2},
		{"online node", primary(func(n *model.Node) { n.Status = "online" }), []*model.Node{backup}, 0},
		{"never heard from", primary(func(n *model.Node) { n.LastHeartbeat = 0 }), []*model.Node{backup}, 0},
		{"no failover configured", primary(func(n *model.Node) { n.FailoverNodeId = 0 }), []*model.Node{backup}, 0},
		{"backup missing", primary(nil), nil, 0},
		{"backup offline too", primary(nil), []*model.Node{{Id: 2, Address: "b", Enable: true, Status: "offline"}}, 0},
		{"backup disabled", primary(nil), []*model.Node{{Id: 2, Address: "b", Enable: false, Status: "online"}}, 0},
		{"backup without an address", primary(nil), []*model.Node{{Id: 2, Enable: true, Status: "online"}}, 0},
		{"points at itself", primary(func(n *model.Node) { n.FailoverNodeId = 1 }), []*model.Node{backup}, 0},
	}
	for _, tc := range cases {
		byID := map[int]*model.Node{tc.node.Id: tc.node}
		for _, o := range tc.others {
			byID[o.Id] = o
		}
		got := NodeFailoverTarget(tc.node, byID, now)
		switch {
		case tc.want == 0 && got != nil:
			t.Errorf("%s: failed over to node %d, want none", tc.name, got.Id)
		case tc.want != 0 && (got == nil || got.Id != tc.want):
			t.Errorf("%s: target = %v, want node %d", tc.name, got, tc.want)
		}
	}
}

func TestNodeFailoverSettingsAreValidatedAndStored(t *testing.T) {
	setupBulkDB(t)
	svc := &NodeService{}
	backup := &model.Node{Name: "backup", Address: "b.example.com", Port: 2053, ApiToken: "t", Enable: true}
	if err := svc.Create(backup); err != nil {
		t.Fatal(err)
	}

	primary := &model.Node{Name: "primary", Address: "a.example.com", Port: 2053, ApiToken: "t", Enable: true, FailoverNodeId: backup.Id}
	if err := svc.Create(primary); err != nil {
		t.Fatalf("Create with failover: %v", err)
	}
	var stored model.Node
	database.GetDB().First(&stored, primary.Id)
	if stored.FailoverNodeId != backup.Id || stored.FailoverAfterSec != 120 {
		t.Fatalf("stored failover = node %d after %ds, want node %d after the 120s default", stored.FailoverNodeId, stored.FailoverAfterSec, backup.Id)
	}

	for name, mod := range map[string]func(*model.Node){
		"unknown backup node":    func(n *model.Node) { n.FailoverNodeId = 999 },
		"grace period too short": func(n *model.Node) { n.FailoverAfterSec = 5 },
		"grace period too long":  func(n *model.Node) { n.FailoverAfterSec = 90000 },
	} {
		bad := &model.Node{Name: "bad-" + name, Address: "c.example.com", Port: 2053, ApiToken: "t", Enable: true, FailoverNodeId: backup.Id, FailoverAfterSec: 300}
		mod(bad)
		if err := svc.Create(bad); err == nil {
			t.Errorf("%s: Create succeeded", name)
		}
	}

	self := stored
	self.FailoverNodeId = primary.Id
	self.ApiToken = "t"
	if err := svc.Update(primary.Id, &self); err == nil {
		t.Fatal("a node was allowed to fail over to itself")
	}
}
