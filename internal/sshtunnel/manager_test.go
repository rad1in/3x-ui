package sshtunnel

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestManagerAppliesAccountChangesWithoutDroppingTheListener(t *testing.T) {
	m := &Manager{procs: map[int]*managed{}, startErrs: map[int]string{}}
	t.Cleanup(m.StopAll)
	inst := Instance{Id: 7, Tag: "ssh-7", Listen: "127.0.0.1", Port: freePort(t), HostKey: hostKey(t), Users: users()}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	before := m.procs[7].srv
	c, err := dial(t, before, "alice@x", "alice-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	inst.Users = map[string]User{"alice@x": {Password: "alice-pass"}, "carol@x": {Password: "carol-pass"}}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if m.procs[7].srv != before {
		t.Fatal("an account change restarted the server")
	}
	if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
		t.Fatalf("alice was dropped by an unrelated account change: %v", err)
	}
	if carol, err := dial(t, before, "carol@x", "carol-pass"); err != nil {
		t.Fatalf("the new account cannot log in: %v", err)
	} else {
		_ = carol.Close()
	}

	inst.Banner = "welcome"
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if m.procs[7].srv == before {
		t.Fatal("a banner change did not rebuild the server")
	}
}

func TestManagerReportsWhyAnInboundDoesNotRun(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	m := &Manager{procs: map[int]*managed{}, startErrs: map[int]string{}}
	port := busy.Addr().(*net.TCPAddr).Port
	err = m.Ensure(Instance{Id: 3, Listen: "127.0.0.1", Port: port, HostKey: hostKey(t), Users: users()})
	if err == nil {
		t.Fatal("started on a busy port")
	}
	running, reason := m.Status(3)
	if running || !strings.Contains(reason, strconv.Itoa(port)) {
		t.Fatalf("status = %v, %q; want not running with the bind error", running, reason)
	}
}

func TestReconcileStopsInboundsThatAreNoLongerWanted(t *testing.T) {
	m := &Manager{procs: map[int]*managed{}, startErrs: map[int]string{}}
	t.Cleanup(m.StopAll)
	port := freePort(t)
	inst := Instance{Id: 1, Listen: "127.0.0.1", Port: port, HostKey: hostKey(t), Users: users()}
	m.Reconcile([]Instance{inst})
	if running, _ := m.Status(1); !running {
		t.Fatal("not running after reconcile")
	}
	m.Reconcile(nil)
	if running, _ := m.Status(1); running {
		t.Fatal("still running after it was dropped")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			_ = ln.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the port was not released: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestInstanceFromInboundServesOnlyUsableAccounts(t *testing.T) {
	key := hostKey(t)
	settings := `{"hostKey":` + strconv.Quote(key) + `,"clients":[` +
		`{"email":"on","password":"p1","enable":true,"limitIp":2},` +
		`{"email":"off","password":"p2","enable":false},` +
		`{"email":"nopass","password":"","enable":true}]}`
	ib := &model.Inbound{Id: 5, Protocol: model.SSH, Port: 2222, Tag: "ssh-2222", Settings: settings}
	inst, ok := InstanceFromInbound(ib)
	if !ok || len(inst.Users) != 1 || inst.Users["on"] != (User{Password: "p1", MaxConns: 2}) {
		t.Fatalf("instance = %+v, ok = %v; want only the enabled account with a password", inst, ok)
	}
	ib.Settings = `{"clients":[{"email":"on","password":"p1","enable":true}]}`
	if _, ok := InstanceFromInbound(ib); ok {
		t.Fatal("an inbound without a host key was served")
	}
}
