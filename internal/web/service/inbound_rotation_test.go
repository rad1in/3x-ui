package service

import (
	"encoding/json"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const rotationRealityStream = `{"network":"tcp","security":"reality","realitySettings":{"show":false,"target":"www.microsoft.com:443","xver":0,"serverNames":["www.microsoft.com"],"privateKey":"k","shortIds":["ab"],"settings":{"publicKey":"p","fingerprint":"chrome"}}}`

const rotationClients = `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"alice@x","enable":true,"flow":""}],"decryption":"none"}`

func realityInbound(t *testing.T, port int) *model.Inbound {
	t.Helper()
	return mkInboundStream(t, port, model.VLESS, rotationClients, rotationRealityStream)
}

func allPortsFree(t *testing.T) {
	t.Helper()
	prev := localPortFree
	localPortFree = func(int, transportBits) bool { return true }
	t.Cleanup(func() { localPortFree = prev })
}

func reality(t *testing.T, stream string) (target string, serverNames []string) {
	t.Helper()
	var s struct {
		RealitySettings struct {
			Target      string   `json:"target"`
			Dest        string   `json:"dest"`
			ServerNames []string `json:"serverNames"`
		} `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(stream), &s); err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	if s.RealitySettings.Dest != "" {
		t.Errorf("stale dest %q left beside target", s.RealitySettings.Dest)
	}
	return s.RealitySettings.Target, s.RealitySettings.ServerNames
}

func TestSaveRotationRejectsInvalidSetups(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	rb := realityInbound(t, 8443)
	plain := mkInbound(t, 8444, model.VLESS, rotationClients)

	cases := []struct {
		name string
		cfg  model.InboundRotation
	}{
		{"unknown inbound", model.InboundRotation{InboundId: 999, RotatePort: true, PortMin: 20000, PortMax: 30000}},
		{"nothing selected", model.InboundRotation{InboundId: rb.Id}},
		{"port min above max", model.InboundRotation{InboundId: rb.Id, RotatePort: true, PortMin: 30000, PortMax: 20000}},
		{"port above 65535", model.InboundRotation{InboundId: rb.Id, RotatePort: true, PortMin: 20000, PortMax: 70000}},
		{"port below 1", model.InboundRotation{InboundId: rb.Id, RotatePort: true, PortMin: 0, PortMax: 100}},
		{"sni without list", model.InboundRotation{InboundId: rb.Id, RotateSNI: true}},
		{"sni entry not a hostname", model.InboundRotation{InboundId: rb.Id, RotateSNI: true, SniList: "www.apple.com\n10.0.0.1"}},
		{"sni entry bad port", model.InboundRotation{InboundId: rb.Id, RotateSNI: true, SniList: "www.apple.com:99999"}},
		{"sni on a non-reality inbound", model.InboundRotation{InboundId: plain.Id, RotateSNI: true, SniList: "www.apple.com"}},
		{"interval above a year", model.InboundRotation{InboundId: rb.Id, RotatePort: true, PortMin: 20000, PortMax: 30000, IntervalHours: 9000}},
		{"negative interval", model.InboundRotation{InboundId: rb.Id, RotatePort: true, PortMin: 20000, PortMax: 30000, IntervalHours: -1}},
	}
	for _, tc := range cases {
		if _, err := svc.Save(&tc.cfg); err == nil {
			t.Errorf("%s: Save succeeded, want an error", tc.name)
		}
	}
}

func TestSaveRotationKeepsLastRotationFields(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	ib := realityInbound(t, 8443)
	cfg := &model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 30000, IntervalHours: 6}
	if _, err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&model.InboundRotation{}).Where("inbound_id = ?", ib.Id).
		Updates(map[string]any{"last_rotated_at": int64(12345), "last_port": 25000})

	saved, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 21000, PortMax: 31000, IntervalHours: 12, LastRotatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if saved.PortMin != 21000 || saved.IntervalHours != 12 {
		t.Fatalf("settings not updated: %+v", saved)
	}
	if saved.LastRotatedAt != 12345 || saved.LastPort != 25000 {
		t.Fatalf("a client-supplied or reset last-rotation state replaced the stored one: %+v", saved)
	}
	var rows int64
	database.GetDB().Model(&model.InboundRotation{}).Where("inbound_id = ?", ib.Id).Count(&rows)
	if rows != 1 {
		t.Fatalf("rows = %d, want the existing row updated", rows)
	}
}

func TestRotateMovesPortAndSNIAndKeepsClients(t *testing.T) {
	setupBulkDB(t)
	allPortsFree(t)
	svc := &InboundRotationService{}
	ib := realityInbound(t, 8443)
	if _, err := svc.Save(&model.InboundRotation{
		InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 20050,
		RotateSNI: true, SniList: "www.microsoft.com, www.apple.com:8443\nwww.samsung.com",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Rotate(ib.Id, time.UnixMilli(1760000000000))
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if res.OldPort != 8443 || res.NewPort < 20000 || res.NewPort > 20050 {
		t.Fatalf("result = %+v, want a port inside 20000-20050", res)
	}

	var got model.Inbound
	if err := database.GetDB().First(&got, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.Port != res.NewPort {
		t.Fatalf("stored port = %d, want %d", got.Port, res.NewPort)
	}
	target, names := reality(t, got.StreamSettings)
	if res.NewSni == "www.microsoft.com" || !slices.Contains([]string{"www.apple.com", "www.samsung.com"}, res.NewSni) {
		t.Fatalf("new SNI = %q, want one of the other list entries", res.NewSni)
	}
	if !slices.Equal(names, []string{res.NewSni}) {
		t.Fatalf("serverNames = %v, want [%s]", names, res.NewSni)
	}
	wantTarget := res.NewSni + ":443"
	if res.NewSni == "www.apple.com" {
		wantTarget = "www.apple.com:8443"
	}
	if target != wantTarget {
		t.Fatalf("target = %q, want %q", target, wantTarget)
	}
	var stream map[string]any
	_ = json.Unmarshal([]byte(got.StreamSettings), &stream)
	rs := stream["realitySettings"].(map[string]any)
	if rs["privateKey"] != "k" || stream["network"] != "tcp" {
		t.Fatalf("rotation dropped other stream settings: %v", stream)
	}
	var settings struct {
		Clients []struct{ Email string } `json:"clients"`
	}
	_ = json.Unmarshal([]byte(got.Settings), &settings)
	if len(settings.Clients) != 1 || settings.Clients[0].Email != "alice@x" {
		t.Fatalf("clients changed by rotation: %s", got.Settings)
	}

	cfg, _ := svc.Get(ib.Id)
	if cfg.LastRotatedAt != 1760000000000 || cfg.LastPort != res.NewPort || cfg.LastSni != res.NewSni {
		t.Fatalf("last rotation not recorded: %+v", cfg)
	}
}

func TestRotateSkipsTakenPortsAndFailsCleanlyWhenNoneFree(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	ib := realityInbound(t, 8443)
	mkInbound(t, 20001, model.VLESS, rotationClients) // another inbound holds 20001
	if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 20002}); err != nil {
		t.Fatal(err)
	}

	taken := map[int]bool{20000: true}
	prev := localPortFree
	localPortFree = func(p int, _ transportBits) bool { return !taken[p] }
	t.Cleanup(func() { localPortFree = prev })

	res, err := svc.Rotate(ib.Id, time.Now())
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if res.NewPort != 20002 {
		t.Fatalf("port = %d, want 20002 (20000 busy on the host, 20001 used by another inbound)", res.NewPort)
	}

	taken[20002] = true
	before := mustPort(t, ib.Id)
	if _, err := svc.Rotate(ib.Id, time.Now()); err == nil {
		t.Fatal("Rotate succeeded with no free port")
	}
	if after := mustPort(t, ib.Id); after != before {
		t.Fatalf("a failed rotation moved the port from %d to %d", before, after)
	}
}

func mustPort(t *testing.T, id int) int {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, id).Error; err != nil {
		t.Fatal(err)
	}
	return ib.Port
}

// Configs imported from elsewhere carry the destination as "dest"; xray reads
// whichever is present, so a stale one beside the new target would win.
func TestRotateSNIReplacesALegacyDestKey(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	legacy := `{"network":"tcp","security":"reality","realitySettings":{"dest":"www.microsoft.com:443","serverNames":["www.microsoft.com"],"privateKey":"k"}}`
	ib := mkInboundStream(t, 8443, model.VLESS, rotationClients, legacy)
	if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotateSNI: true, SniList: "www.apple.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rotate(ib.Id, time.Now()); err != nil {
		t.Fatal(err)
	}
	var got model.Inbound
	database.GetDB().First(&got, ib.Id)
	target, names := reality(t, got.StreamSettings)
	if target != "www.apple.com:443" || !slices.Equal(names, []string{"www.apple.com"}) {
		t.Fatalf("target %q serverNames %v", target, names)
	}
}

func TestRotateSNIOnlyNeedsADifferentEntry(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	ib := realityInbound(t, 8443)
	if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotateSNI: true, SniList: "www.microsoft.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rotate(ib.Id, time.Now()); err == nil {
		t.Fatal("rotating to the SNI already in use succeeded")
	}
}

func TestRotateWithoutSavedSetupFails(t *testing.T) {
	setupBulkDB(t)
	ib := realityInbound(t, 8443)
	if _, err := (&InboundRotationService{}).Rotate(ib.Id, time.Now()); err == nil {
		t.Fatal("Rotate without a saved setup succeeded")
	}
}

// A node inbound is edited like any other: the master saves the new port and
// marks the node dirty, and the node sync pushes the stored inbound to it.
func TestRotateOnANodeInboundMarksTheNodeForSync(t *testing.T) {
	setupBulkDB(t)
	useTestRuntimeManager(t)
	node := &model.Node{Name: "n-rot", Address: "127.0.0.1", Port: 2096, ApiToken: "tok", Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	// A node's ports are not this host's: a busy local port must not matter.
	prev := localPortFree
	localPortFree = func(int, transportBits) bool { return false }
	t.Cleanup(func() { localPortFree = prev })

	ib := &model.Inbound{UserId: 1, NodeID: &node.Id, Tag: "node-reality", Enable: true, Port: 8443, Protocol: model.VLESS,
		Settings: rotationClients, StreamSettings: rotationRealityStream}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundRotationService{}
	if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 30000, PortMax: 30100}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Rotate(ib.Id, time.Now())
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	var stored model.Inbound
	database.GetDB().First(&stored, ib.Id)
	if stored.Port != res.NewPort || stored.NodeID == nil || *stored.NodeID != node.Id {
		t.Fatalf("stored inbound = port %d node %v, want port %d on node %d", stored.Port, stored.NodeID, res.NewPort, node.Id)
	}
	var n model.Node
	database.GetDB().First(&n, node.Id)
	if !n.ConfigDirty {
		t.Fatal("the node was not marked dirty, so nothing would push the new port to it")
	}
}

func TestTickRotatesOnlyDueInbounds(t *testing.T) {
	setupBulkDB(t)
	allPortsFree(t)
	svc := &InboundRotationService{}
	due := realityInbound(t, 8443)
	fresh := mkInboundStream(t, 8444, model.VLESS, rotationClients, rotationRealityStream)
	manual := mkInboundStream(t, 8445, model.VLESS, rotationClients, rotationRealityStream)
	now := time.UnixMilli(1760000000000)
	for _, ib := range []*model.Inbound{due, fresh, manual} {
		interval := 6
		if ib.Id == manual.Id {
			interval = 0
		}
		if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 20400, IntervalHours: interval}); err != nil {
			t.Fatal(err)
		}
	}
	setNext := func(id int, at time.Time) {
		database.GetDB().Model(&model.InboundRotation{}).Where("inbound_id = ?", id).Update("next_rotation_at", at.UnixMilli())
	}
	setNext(due.Id, now.Add(-time.Minute))
	setNext(fresh.Id, now.Add(time.Hour))
	setNext(manual.Id, now.Add(-time.Hour)) // a stale value must not run a manual-only setup

	results := svc.Tick(now)
	if len(results) != 1 || results[0].InboundId != due.Id {
		t.Fatalf("rotated %+v, want exactly the overdue scheduled inbound", results)
	}
	if mustPort(t, fresh.Id) != 8444 || mustPort(t, manual.Id) != 8445 {
		t.Fatal("an inbound that was not due changed port")
	}
	cfg, _ := svc.Get(due.Id)
	if want := now.Add(6 * time.Hour).UnixMilli(); cfg.NextRotationAt != want {
		t.Fatalf("next rotation = %d, want %d (one interval after this rotation)", cfg.NextRotationAt, want)
	}
	if again := svc.Tick(now); len(again) != 0 {
		t.Fatalf("the same tick rotated again: %+v", again)
	}
}

func TestSaveStartsTheScheduleClockOnlyWhenTheIntervalChanges(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	ib := realityInbound(t, 8443)
	cfg := model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 30000, IntervalHours: 6}

	before := time.Now()
	first, err := svc.Save(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := before.Add(6*time.Hour).UnixMilli(), time.Now().Add(6*time.Hour).UnixMilli()
	if first.NextRotationAt < lo || first.NextRotationAt > hi {
		t.Fatalf("next rotation = %d, want about six hours from now (%d..%d)", first.NextRotationAt, lo, hi)
	}

	cfg.PortMax = 31000 // an unrelated edit keeps the running clock
	same, _ := svc.Save(&cfg)
	if same.NextRotationAt != first.NextRotationAt {
		t.Fatalf("an edit that kept the interval moved the clock: %d -> %d", first.NextRotationAt, same.NextRotationAt)
	}
	cfg.IntervalHours = 12
	longer, _ := svc.Save(&cfg)
	if longer.NextRotationAt < time.Now().Add(11*time.Hour).UnixMilli() {
		t.Fatalf("a new interval did not restart the clock: next rotation %d", longer.NextRotationAt)
	}
	cfg.IntervalHours = 0
	off, _ := svc.Save(&cfg)
	if off.NextRotationAt != 0 {
		t.Fatalf("manual-only setup has next rotation %d", off.NextRotationAt)
	}
}

func TestTickSurvivesAFailingInbound(t *testing.T) {
	setupBulkDB(t)
	allPortsFree(t)
	svc := &InboundRotationService{}
	good := realityInbound(t, 8443)
	gone := realityInbound(t, 8444)
	for _, ib := range []*model.Inbound{gone, good} {
		if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 20400, IntervalHours: 1}); err != nil {
			t.Fatal(err)
		}
	}
	database.GetDB().Model(&model.InboundRotation{}).Where("1 = 1").Update("next_rotation_at", 1)
	database.GetDB().Delete(&model.Inbound{}, gone.Id)
	results := svc.Tick(time.Now())
	if len(results) != 1 || results[0].InboundId != good.Id {
		t.Fatalf("results = %+v, want only the surviving inbound", results)
	}
}

func TestLocalPortFree(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if localPortFree(port, transportTCP) {
		t.Fatal("a listening TCP port reported free")
	}
	_ = ln.Close()
	if !localPortFree(port, transportTCP) {
		t.Fatal("a closed TCP port reported busy")
	}
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	if localPortFree(pc.LocalAddr().(*net.UDPAddr).Port, transportUDP) {
		t.Fatal("a bound UDP port reported free")
	}
}

func TestDeletingAnInboundDropsItsRotation(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundRotationService{}
	ib := &model.Inbound{Tag: "casc", Enable: false, Port: 4443, Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: rotationRealityStream}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(&model.InboundRotation{InboundId: ib.Id, RotatePort: true, PortMin: 20000, PortMax: 30000}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&InboundService{}).DelInbound(ib.Id); err != nil {
		t.Fatal(err)
	}
	var rows int64
	database.GetDB().Model(&model.InboundRotation{}).Where("inbound_id = ?", ib.Id).Count(&rows)
	if rows != 0 {
		t.Fatalf("rotation rows left after the inbound was deleted: %d", rows)
	}
}
