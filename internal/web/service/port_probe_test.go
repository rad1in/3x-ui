package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

type probeLog struct {
	mu      sync.Mutex
	targets []portProbeTarget
	fail    map[int]bool
}

func (l *probeLog) probe(_ context.Context, tg portProbeTarget) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.targets = append(l.targets, tg)
	if l.fail[tg.InboundId] {
		return errors.New("connection timed out")
	}
	return nil
}

func probeService(t *testing.T, log *probeLog) *PortProbeService {
	t.Helper()
	setupBulkDB(t)
	s := &PortProbeService{probe: log.probe, publicHost: func() string { return "203.0.113.7" }}
	if err := s.settings.setString("portProbeEnable", "true"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return s
}

func eventTypes(evs []eventbus.Event) []eventbus.EventType {
	out := make([]eventbus.EventType, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

func TestPortProbeAlertsOnceAfterConsecutiveFailuresAndOnRecovery(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	ib := mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	log.fail[ib.Id] = true

	// Default threshold is 3 consecutive failures.
	for i := 1; i <= 2; i++ {
		if evs := s.Tick(); len(evs) != 0 {
			t.Fatalf("tick %d published %v before the threshold", i, eventTypes(evs))
		}
	}
	evs := s.Tick()
	if !slices.Equal(eventTypes(evs), []eventbus.EventType{eventbus.EventPortBlocked}) {
		t.Fatalf("tick 3 events = %v, want one port.blocked", eventTypes(evs))
	}
	data, ok := evs[0].Data.(*eventbus.PortProbeData)
	if !ok || data.InboundId != ib.Id || data.Port != 8443 || data.Host != "203.0.113.7" || data.Method != "tcp" || data.Error == "" {
		t.Fatalf("event data = %+v", evs[0].Data)
	}
	if evs := s.Tick(); len(evs) != 0 {
		t.Fatalf("a continuing outage published %v again", eventTypes(evs))
	}

	log.fail[ib.Id] = false
	evs = s.Tick()
	if !slices.Equal(eventTypes(evs), []eventbus.EventType{eventbus.EventPortRestored}) {
		t.Fatalf("recovery events = %v, want one port.restored", eventTypes(evs))
	}
	if evs := s.Tick(); len(evs) != 0 {
		t.Fatalf("steady state published %v", eventTypes(evs))
	}
}

func TestPortProbeSingleFailureBetweenSuccessesNeverAlerts(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	ib := mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	for _, fail := range []bool{true, true, false, true, true, false} {
		log.fail[ib.Id] = fail
		if evs := s.Tick(); len(evs) != 0 {
			t.Fatalf("non-consecutive failures published %v", eventTypes(evs))
		}
	}
}

func TestPortProbeFailureThresholdIsConfigurable(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	if err := s.settings.setString("portProbeFailures", "1"); err != nil {
		t.Fatal(err)
	}
	ib := mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	log.fail[ib.Id] = true
	if evs := s.Tick(); len(evs) != 1 {
		t.Fatalf("threshold 1: events = %v, want one", eventTypes(evs))
	}
}

func TestPortProbeDisabledProbesNothing(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	_ = s.settings.setString("portProbeEnable", "false")
	mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	if evs := s.Tick(); len(evs) != 0 || len(log.targets) != 0 {
		t.Fatalf("disabled probe ran: events %v, targets %v", eventTypes(evs), log.targets)
	}
}

func TestPortProbeTargets(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	db := database.GetDB()

	public := mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	mkInbound(t, 8444, model.Hysteria, `{"clients":[]}`) // UDP only
	loop := mkInbound(t, 8445, model.VMESS, `{"clients":[]}`)
	db.Model(loop).Update("listen", "127.0.0.1")
	sock := mkInbound(t, 8446, model.VLESS, `{"clients":[]}`)
	db.Model(sock).Update("listen", "/dev/shm/in.sock")
	off := mkInbound(t, 8447, model.Trojan, `{"clients":[]}`)
	db.Model(off).Update("enable", false)
	kcp := mkInboundStream(t, 8448, model.VLESS, `{"clients":[]}`, `{"network":"kcp"}`)

	online := &model.Node{Name: "fra", Address: "node1.example.com", Port: 2053, Enable: true, Status: "online"}
	down := &model.Node{Name: "ams", Address: "node2.example.com", Port: 2053, Enable: true, Status: "offline"}
	db.Create(online)
	db.Create(down)
	onNode := mkInbound(t, 9443, model.VLESS, `{"clients":[]}`)
	db.Model(onNode).Update("node_id", online.Id)
	onDownNode := mkInbound(t, 9444, model.VLESS, `{"clients":[]}`)
	db.Model(onDownNode).Update("node_id", down.Id)

	s.Tick()

	got := map[int]string{}
	for _, tg := range log.targets {
		got[tg.InboundId] = tg.Host
	}
	want := map[int]string{public.Id: "203.0.113.7", onNode.Id: "node1.example.com"}
	if len(got) != len(want) || got[public.Id] != want[public.Id] || got[onNode.Id] != want[onNode.Id] {
		t.Fatalf("probed %v, want exactly %v (skipped: UDP, loopback, socket, disabled, kcp %d, offline node)", got, want, kcp.Id)
	}
}

func TestPortProbeConfiguredHostOverridesPublicIP(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	_ = s.settings.setString("portProbeHost", "vpn.example.com")
	mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	s.Tick()
	if len(log.targets) != 1 || log.targets[0].Host != "vpn.example.com" {
		t.Fatalf("targets = %+v, want the configured host", log.targets)
	}
}

func TestPortProbeForgetsRemovedInbounds(t *testing.T) {
	log := &probeLog{fail: map[int]bool{}}
	s := probeService(t, log)
	ib := mkInbound(t, 8443, model.VLESS, `{"clients":[]}`)
	log.fail[ib.Id] = true
	s.Tick()
	s.Tick()
	database.GetDB().Delete(&model.Inbound{}, ib.Id)
	s.Tick()
	// A new inbound reusing the id must start from zero failures.
	again := &model.Inbound{Id: ib.Id, Tag: "again", Port: 8443, Protocol: model.VLESS, Enable: true, Settings: `{"clients":[]}`}
	database.GetDB().Create(again)
	if evs := s.Tick(); len(evs) != 0 {
		t.Fatalf("stale failure count carried over to a new inbound: %v", eventTypes(evs))
	}
}

func TestProbeURLSubstitutesTargetAndChecksResponse(t *testing.T) {
	var gotQuery string
	body := "open"
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tg := portProbeTarget{Host: "203.0.113.7", Port: 8443}
	cfg := portProbeConfig{URL: srv.URL + "/check?host={host}&port={port}", Expect: "open"}
	check := func() error { return probeByURL(context.Background(), srv.Client(), cfg, tg) }

	if err := check(); err != nil {
		t.Fatalf("open response: %v", err)
	}
	if gotQuery != "host=203.0.113.7&port=8443" {
		t.Fatalf("query = %q", gotQuery)
	}
	body = "closed"
	if err := check(); err == nil {
		t.Fatal("a body without the expected text counted as open")
	}
	body, status = "open", http.StatusBadGateway
	if err := check(); err == nil {
		t.Fatal("a 502 counted as open")
	}
	cfg.Expect = ""
	status, body = http.StatusOK, "anything"
	if err := check(); err != nil {
		t.Fatalf("no expected text, 200: %v", err)
	}
}

func TestProbeByTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	if err := probeByTCP(context.Background(), portProbeTarget{Host: "127.0.0.1", Port: port}); err != nil {
		t.Fatalf("listening port: %v", err)
	}
	_ = ln.Close()
	if err := probeByTCP(context.Background(), portProbeTarget{Host: "127.0.0.1", Port: port}); err == nil {
		t.Fatal("closed port answered")
	}
}
