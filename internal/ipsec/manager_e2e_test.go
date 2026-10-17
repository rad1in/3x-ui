package ipsec

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

const (
	e2eNS      = "xuike"
	serverAddr = "192.168.78.1"
	clientAddr = "192.168.78.2"
	// A second server address, so client traffic has somewhere to go through the tunnel.
	targetAddr = "192.168.79.1"
)

type noRouting struct{}

func (noRouting) Up(netip.Prefix) error { return nil }
func (noRouting) Down(netip.Prefix)     {}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func setupNamespace(t *testing.T) {
	t.Helper()
	_ = exec.Command("ip", "netns", "del", e2eNS).Run()
	run(t, "ip", "netns", "add", e2eNS)
	run(t, "ip", "link", "add", "xuike0", "type", "veth", "peer", "name", "xuike1")
	t.Cleanup(func() {
		_ = exec.Command("ip", "link", "del", "xuike0").Run()
		_ = exec.Command("ip", "netns", "del", e2eNS).Run()
	})
	run(t, "ip", "link", "set", "xuike1", "netns", e2eNS)
	run(t, "ip", "addr", "add", serverAddr+"/24", "dev", "xuike0")
	run(t, "ip", "addr", "add", targetAddr+"/32", "dev", "xuike0")
	run(t, "ip", "link", "set", "xuike0", "up")
	run(t, "ip", "-n", e2eNS, "addr", "add", clientAddr+"/24", "dev", "xuike1")
	run(t, "ip", "-n", e2eNS, "link", "set", "xuike1", "up")
	run(t, "ip", "-n", e2eNS, "link", "set", "lo", "up")
}

// client is a strongSwan road warrior in the test namespace, with its own
// /var/run so its pid file does not clash with the server's.
type client struct {
	t   *testing.T
	dir string
	uri string
}

func startClient(t *testing.T, bin string, ca string) *client {
	t.Helper()
	dir := t.TempDir()
	c := &client{t: t, dir: dir, uri: "unix://" + filepath.Join(dir, "charon.vici")}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ca.pem", ca)
	write("strongswan.conf", fmt.Sprintf(`charon {
  plugins {
    vici { socket = %s }
    kernel-libipsec { load = %v }
    bypass-lan { load = no }
    stroke { load = no }
  }
}
`, c.uri, map[bool]string{true: "no", false: "yes"}[kernelESP()]))
	cmd := exec.CommandContext(context.Background(), "ip", "netns", "exec", e2eNS, "unshare", "-m", "sh", "-c",
		"mount -t tmpfs tmpfs /var/run && exec "+bin)
	cmd.Env = append(os.Environ(), "STRONGSWAN_CONF="+filepath.Join(dir, "strongswan.conf"))
	if testing.Verbose() {
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	waitFor(t, "client charon", 15*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(dir, "charon.vici"))
		return err == nil
	})
	return c
}

// login loads user/password and tries to connect, reporting whether it did.
func (c *client) login(user, password string) bool {
	c.t.Helper()
	conf := fmt.Sprintf(`connections {
  home {
    version = 2
    remote_addrs = %s
    vips = 0.0.0.0
    local { auth = eap-mschapv2
            eap_id = %s }
    remote { auth = pubkey
             id = %s
             cacerts = %s }
    children { home { remote_ts = %s/32 } }
  }
}
secrets { eap-user { id = %s
                     secret = "%s" } }
`, serverAddr, user, serverAddr, filepath.Join(c.dir, "ca.pem"), targetAddr, user, password)
	path := filepath.Join(c.dir, "swanctl.conf")
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		c.t.Fatal(err)
	}
	_ = exec.Command("swanctl", "--terminate", "--ike", "home", "--force", "--uri", c.uri).Run()
	_ = exec.Command("swanctl", "--load-creds", "--clear", "--file", path, "--uri", c.uri).Run()
	run(c.t, "swanctl", "--load-conns", "--file", path, "--uri", c.uri)
	return exec.Command("swanctl", "--initiate", "--child", "home", "--timeout", "20", "--uri", c.uri).Run() == nil
}

func (c *client) connected() bool {
	out, _ := exec.Command("swanctl", "--list-sas", "--uri", c.uri).CombinedOutput()
	return strings.Contains(string(out), "ESTABLISHED")
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

// TestIKEv2EndToEnd runs the panel's charon against a real strongSwan client.
// It needs root, ip netns, unshare and IKEV2_E2E=1.
func TestIKEv2EndToEnd(t *testing.T) {
	if os.Getenv("IKEV2_E2E") == "" {
		t.Skip("IKEV2_E2E not set")
	}
	bin, err := BinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	prev := vpnnat.Default
	vpnnat.Default = noRouting{}
	t.Cleanup(func() { vpnnat.Default = prev })
	setupNamespace(t)

	pki, err := PKIFor(PKI{}, serverAddr)
	if err != nil {
		t.Fatal(err)
	}
	inst := Instance{
		Id: 1, Tag: "ike-e2e", Address: serverAddr, Pool: netip.MustParsePrefix("10.89.200.0/24"),
		DNS: []string{"1.1.1.1"}, PKI: pki, Users: map[string]User{"alice": {Password: "first-pass"}},
	}
	m := newManager()
	if err := m.Reconcile([]Instance{inst}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.StopAll)

	echo, err := net.ListenPacket("udp4", targetAddr+":0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := echo.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteTo(buf[:n], from)
		}
	}()
	port := echo.LocalAddr().(*net.UDPAddr).Port

	var up, down int64
	var online []string
	collect := func() {
		traffic, on := m.CollectTraffic()
		for _, tr := range traffic {
			if tr.Tag == "ike-e2e" && tr.Email == "alice" {
				up += tr.Up
				down += tr.Down
			}
		}
		online = on
	}

	c := startClient(t, bin, pki.CA)
	if !c.login("alice", "first-pass") {
		t.Fatal("alice could not connect with her password")
	}
	waitFor(t, "alice online", 15*time.Second, func() bool { collect(); return slices.Contains(online, "alice") })
	waitFor(t, "alice's traffic metered", 20*time.Second, func() bool {
		_ = exec.Command("ip", "netns", "exec", e2eNS, "bash", "-c",
			fmt.Sprintf("echo panel-metering-probe > /dev/udp/%s/%d", targetAddr, port)).Run()
		collect()
		return up > 0 && down > 0
	})

	// Taking the account away must end the live session and refuse the next login.
	inst.Users = map[string]User{}
	if err := m.Reconcile([]Instance{inst}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "revoked session dropped", 20*time.Second, func() bool { return !c.connected() })
	if c.login("alice", "first-pass") {
		t.Fatal("a revoked account logged in again")
	}

	inst.Users = map[string]User{"alice": {Password: "second-pass"}}
	if err := m.Reconcile([]Instance{inst}); err != nil {
		t.Fatal(err)
	}
	if c.login("alice", "first-pass") {
		t.Fatal("the old password still works")
	}
	if !c.login("alice", "second-pass") {
		t.Fatal("alice could not connect with her new password")
	}
	t.Logf("metered up=%d down=%d", up, down)
}
