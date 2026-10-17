package openvpn

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type noRouting struct{}

func (noRouting) up(Instance) error { return nil }
func (noRouting) down(Instance)     {}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// startClient runs a real openvpn client against the profile the panel hands out.
func startClient(t *testing.T, inst Instance, dev, user, pass string) (*exec.Cmd, <-chan error) {
	t.Helper()
	dir := t.TempDir()
	profile := filepath.Join(dir, "client.ovpn")
	auth := filepath.Join(dir, "auth.txt")
	if err := os.WriteFile(profile, []byte(ClientProfile(inst, "127.0.0.1", "e2e")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auth, []byte(user+"\n"+pass+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The client runs on this host: keep it from taking over the default route.
	cmd := exec.CommandContext(context.Background(), os.Getenv("OPENVPN_E2E_BINARY"), "--config", profile,
		"--auth-user-pass", auth, "--dev", dev, "--pull-filter", "ignore", "redirect-gateway",
		"--pull-filter", "ignore", "dhcp-option", "--verb", "3")
	if testing.Verbose() {
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, done
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestOpenVPNEndToEnd runs real openvpn server and client processes: it needs
// root, /dev/net/tun and OPENVPN_E2E_BINARY pointing at an openvpn binary.
func TestOpenVPNEndToEnd(t *testing.T) {
	bin := os.Getenv("OPENVPN_E2E_BINARY")
	if bin == "" {
		t.Skip("OPENVPN_E2E_BINARY not set")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	if err := os.Symlink(bin, filepath.Join(os.Getenv("XUI_BIN_FOLDER"), "openvpn")); err != nil {
		t.Fatal(err)
	}
	prev := netSetup
	netSetup = noRouting{}
	t.Cleanup(func() { netSetup = prev })

	pki, err := NewPKI("e2e")
	if err != nil {
		t.Fatal(err)
	}
	inst := Instance{
		Id: 901, Tag: "ovpn-e2e", Listen: "127.0.0.1", Port: freeUDPPort(t), Proto: "udp",
		Network: netip.MustParsePrefix("10.199.0.0/24"), PKI: pki,
		Users: map[string]User{"alice": {Password: "right-pass"}},
	}
	m := &Manager{procs: map[int]*managed{}}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.StopAll)

	var up, down int64
	var online []string
	collect := func() {
		traffic, on := m.CollectTraffic()
		for _, tr := range traffic {
			if tr.Email == "alice" && tr.Tag == "ovpn-e2e" {
				up += tr.Up
				down += tr.Down
			}
		}
		online = on
	}

	_, aliceDone := startClient(t, inst, "ovpnc1", "alice", "right-pass")
	waitFor(t, "alice online", 30*time.Second, func() bool { collect(); return slices.Contains(online, "alice") })
	waitFor(t, "alice's traffic metered", 15*time.Second, func() bool { collect(); return up > 0 && down > 0 })

	_, badDone := startClient(t, inst, "ovpnc2", "alice", "wrong-pass")
	select {
	case <-badDone:
	case <-time.After(30 * time.Second):
		t.Fatal("a client with a wrong password was not refused")
	}

	// Disabling the account in the panel must drop the live session.
	inst.Users = map[string]User{}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	select {
	case <-aliceDone:
	case <-time.After(45 * time.Second):
		t.Fatal("a revoked user's session survived")
	}
	waitFor(t, "revoked user offline", 15*time.Second, func() bool { collect(); return !slices.Contains(online, "alice") })
	t.Logf("metered up=%d down=%d", up, down)
}
