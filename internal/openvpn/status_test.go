package openvpn

import (
	"net/netip"
	"strings"
	"testing"
)

// An inbound that failed to start is in no process table; its status must
// still say why, or the panel shows a dead server with no reason.
func TestStatusExplainsAFailedStart(t *testing.T) {
	m := &Manager{procs: map[int]*managed{}}
	inst := Instance{Id: 7, Tag: "ovpn", Port: 1194, Proto: "udp", Network: netip.MustParsePrefix("10.88.0.0/24")}
	if err := m.Ensure(inst); err == nil {
		t.Fatal("an inbound without certificates started")
	}
	running, reason := m.Status(7)
	if running || !strings.Contains(reason, "no certificates") {
		t.Fatalf("status = %v, %q; want not running with the start error", running, reason)
	}
	m.Remove(7)
	if _, reason := m.Status(7); reason != "" {
		t.Fatalf("a removed inbound kept its old error %q", reason)
	}
}
