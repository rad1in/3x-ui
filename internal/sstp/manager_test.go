package sstp

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

type routes struct{ up, down []netip.Prefix }

func (r *routes) Up(n netip.Prefix) error { r.up = append(r.up, n); return nil }
func (r *routes) Down(n netip.Prefix)     { r.down = append(r.down, n) }

func testManager(t *testing.T) (*Manager, *spawns, *routes, string) {
	t.Helper()
	rt := &routes{}
	prev := vpnnat.Default
	vpnnat.Default = rt
	t.Cleanup(func() { vpnnat.Default = prev })
	secrets := filepath.Join(t.TempDir(), "chap-secrets")
	sp := &spawns{}
	m := newManager(sp.spawn, func() error { return nil }, secrets)
	t.Cleanup(m.StopAll)
	return m, sp, rt, secrets
}

func pkiJSON(t *testing.T) string {
	t.Helper()
	bs, err := json.Marshal(pki(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(bs)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func managedInstance(t *testing.T, id int) Instance {
	inst := testInstance(t)
	inst.Id, inst.Port = id, freeTCPPort(t)
	return inst
}

func TestManagerPublishesEachInboundsLoginsToPppdUnderItsOwnName(t *testing.T) {
	m, _, rt, secrets := testManager(t)
	one, two := managedInstance(t, 1), managedInstance(t, 2)
	two.Pool = netip.MustParsePrefix("10.91.1.0/24")
	two.Users = map[string]User{"carol@x": {Password: "pc"}}
	if err := m.Ensure(one); err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(two); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(secrets)
	got := string(raw)
	for _, want := range []string{
		`"alice@x" "x-ui-sstp-1" "pa" *`, `"bob@x" "x-ui-sstp-1" "pb" *`, `"carol@x" "x-ui-sstp-2" "pc" *`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chap-secrets lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"carol@x" "x-ui-sstp-1"`) {
		t.Errorf("carol is valid on inbound 1's pppd too:\n%s", got)
	}
	if len(rt.up) != 2 {
		t.Errorf("routes up = %v, want both client networks", rt.up)
	}

	m.Remove(1)
	raw, _ = os.ReadFile(secrets)
	if strings.Contains(string(raw), "alice@x") || !strings.Contains(string(raw), "carol@x") || len(rt.down) != 1 || rt.down[0] != one.Pool {
		t.Fatalf("after removing inbound 1:\n%s\nroutes down: %v", raw, rt.down)
	}
	m.Remove(2)
	if raw, _ = os.ReadFile(secrets); strings.Contains(string(raw), "x-ui sstp") {
		t.Fatalf("the panel's block survived removing every inbound:\n%s", raw)
	}
}

func TestManagerAppliesAccountChangesWithoutRebuildingTheListener(t *testing.T) {
	m, _, _, secrets := testManager(t)
	inst := managedInstance(t, 1)
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	before := m.procs[1].srv
	c := connect(t, before)
	if !login(t, c, "alice@x") {
		t.Fatal("alice cannot log in")
	}

	inst.Users = map[string]User{"alice@x": {Password: "pa"}, "dave@x": {Password: "pd"}}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if m.procs[1].srv != before {
		t.Fatal("an account change rebuilt the listener")
	}
	raw, _ := os.ReadFile(secrets)
	if !strings.Contains(string(raw), `"dave@x"`) || strings.Contains(string(raw), `"bob@x"`) {
		t.Fatalf("chap-secrets did not follow the change:\n%s", raw)
	}

	inst.DNS = []string{"9.9.9.9"}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if m.procs[1].srv == before {
		t.Fatal("a DNS change did not rebuild the listener")
	}
}

func TestManagerReportsWhyAnInboundDoesNotRun(t *testing.T) {
	m, _, _, _ := testManager(t)
	m.preflight = func() error {
		return errors.New("this kernel has no PPP support (/dev/ppp is missing), which SSTP needs")
	}
	inst := managedInstance(t, 4)
	if err := m.Ensure(inst); err == nil {
		t.Fatal("started without PPP support")
	}
	if running, reason := m.Status(4); running || !strings.Contains(reason, "no PPP support") {
		t.Fatalf("status = %v, %q", running, reason)
	}

	m.preflight = func() error { return nil }
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	inst.Port = busy.Addr().(*net.TCPAddr).Port
	if err := m.Ensure(inst); err == nil {
		t.Fatal("started on a busy port")
	}
	if running, reason := m.Status(4); running || !strings.Contains(reason, strconv.Itoa(inst.Port)) {
		t.Fatalf("status = %v, %q; want the bind error", running, reason)
	}
}

func TestInstanceFromInboundServesOnlyUsableAccountsOfACompleteInbound(t *testing.T) {
	settings := `{"serverAddress":"vpn.example.com","pool":"10.91.4.77/24","dns":["1.1.1.1"],` +
		`"pki":` + pkiJSON(t) + `,"clients":[` +
		`{"email":"on","password":"p1","enable":true,"limitIp":2},` +
		`{"email":"off","password":"p2","enable":false},` +
		`{"email":"nopass","password":"","enable":true}]}`
	ib := &model.Inbound{Id: 5, Protocol: model.SSTP, Port: 443, Tag: "sstp-443", Settings: settings}
	inst, ok := InstanceFromInbound(ib)
	if !ok || len(inst.Users) != 1 || inst.Users["on"] != (User{Password: "p1", MaxConns: 2}) {
		t.Fatalf("instance = %+v, ok = %v; want only the enabled account with a password", inst, ok)
	}
	if inst.Pool != netip.MustParsePrefix("10.91.4.0/24") || inst.Address != "vpn.example.com" {
		t.Fatalf("pool %s, address %s", inst.Pool, inst.Address)
	}
	for name, bad := range map[string]string{
		"no address":   strings.Replace(settings, `"serverAddress":"vpn.example.com"`, `"serverAddress":""`, 1),
		"pool too big": strings.Replace(settings, "10.91.4.77/24", "10.91.0.0/30", 1),
		"no pki":       strings.Replace(settings, `"pki":`+pkiJSON(t), `"pki":{}`, 1),
	} {
		ib.Settings = bad
		if _, ok := InstanceFromInbound(ib); ok {
			t.Errorf("%s: an incomplete inbound was served", name)
		}
	}
}
