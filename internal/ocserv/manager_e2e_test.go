//go:build linux

package ocserv

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

type noRouting struct{}

func (noRouting) Up(netip.Prefix) error { return nil }
func (noRouting) Down(netip.Prefix)     {}

const (
	e2eNamespace = "xuioc"
	e2eHostIP    = "10.99.0.1"
	pingPackets  = 8
	// An echo request or reply of -s 900 is 928 bytes on the tunnel.
	pingBytes = 928
)

func ipCmd(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
		t.Fatalf("ip %v: %v %s", args, err, out)
	}
}

// clientNamespace is where the clients run: a client on the server's own
// host would reach the server's tunnel address locally, not through the tunnel.
func clientNamespace(t *testing.T) {
	t.Helper()
	_ = exec.Command("ip", "netns", "del", e2eNamespace).Run()
	_ = exec.Command("ip", "link", "del", "xuioc0").Run()
	ipCmd(t, "netns", "add", e2eNamespace)
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", e2eNamespace).Run()
		_ = exec.Command("ip", "link", "del", "xuioc0").Run()
	})
	ipCmd(t, "link", "add", "xuioc0", "type", "veth", "peer", "name", "xuioc1")
	ipCmd(t, "link", "set", "xuioc1", "netns", e2eNamespace)
	ipCmd(t, "addr", "add", e2eHostIP+"/24", "dev", "xuioc0")
	ipCmd(t, "link", "set", "xuioc0", "up")
	ipCmd(t, "-n", e2eNamespace, "addr", "add", "10.99.0.2/24", "dev", "xuioc1")
	ipCmd(t, "-n", e2eNamespace, "link", "set", "xuioc1", "up")
}

// vpnClient is one openconnect process connected to the server under test.
type vpnClient struct {
	ifname string
	ip     string
}

func (c *vpnClient) alive() bool {
	return exec.Command("ip", "-n", e2eNamespace, "link", "show", c.ifname).Run() == nil
}

func serverPin(t *testing.T, certPEM string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return "pin-sha256:" + base64.StdEncoding.EncodeToString(sum[:])
}

var configuredAs = regexp.MustCompile(`Configured as ([0-9.]+)`)

// connect logs in with the real openconnect client; a refused login returns an error.
func connect(t *testing.T, inst Instance, user, password, ifname string) (*vpnClient, error) {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	logFile := filepath.Join(dir, "log")
	log, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command("ip", "netns", "exec", e2eNamespace, "openconnect", "--no-proxy", "--user="+user, "--passwd-on-stdin",
		"--servercert", serverPin(t, inst.PKI.ServerCert), "--script", "/bin/true",
		"--background", "--pid-file", pidFile, "-i", ifname, net.JoinHostPort(e2eHostIP, strconv.Itoa(inst.Port)))
	cmd.Stdin = strings.NewReader(password + "\n")
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		out, _ := os.ReadFile(logFile)
		return nil, fmt.Errorf("openconnect: %w: %s", err, out)
	}
	out, _ := os.ReadFile(logFile)
	m := configuredAs.FindSubmatch(out)
	rawPid, _ := os.ReadFile(pidFile)
	pid := strings.TrimSpace(string(rawPid))
	if m == nil || pid == "" {
		return nil, fmt.Errorf("no address in the client log: %s", out)
	}
	t.Cleanup(func() { _ = exec.Command("kill", pid).Run() })
	c := &vpnClient{ifname: ifname, ip: string(m[1])}
	eventually(t, "the client's tunnel device", 10*time.Second, c.alive)
	return c, nil
}

// bringUp gives the client's tunnel device its address and a route to the server's end.
func (c *vpnClient) bringUp(t *testing.T, serverIP string) {
	t.Helper()
	ipCmd(t, "-n", e2eNamespace, "addr", "add", c.ip+"/32", "peer", serverIP, "dev", c.ifname)
	ipCmd(t, "-n", e2eNamespace, "link", "set", c.ifname, "up")
}

// ping sends traffic through the tunnel to the server's end of it.
func (c *vpnClient) ping(t *testing.T, serverIP string) {
	t.Helper()
	cmd := exec.Command("ip", "netns", "exec", e2eNamespace, "ping",
		"-c", strconv.Itoa(pingPackets), "-i", "0.2", "-s", "900", "-I", c.ip, serverIP)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ping through the tunnel: %v\n%s", err, out)
	}
}

func eventually(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A real ocserv and the real openconnect client; needs root, a tun device and
// network namespaces, so it only runs when OCSERV_E2E=1.
func TestRealOcservServesAndMetersClients(t *testing.T) {
	if os.Getenv("OCSERV_E2E") != "1" {
		t.Skip("set OCSERV_E2E=1 (root, ocserv, openconnect, /dev/net/tun, ip netns) to run this")
	}
	for _, tool := range []string{"ocserv", "occtl", "openconnect", "ip", "ping"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	prev := vpnnat.Default
	vpnnat.Default = noRouting{}
	t.Cleanup(func() { vpnnat.Default = prev })
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	clientNamespace(t)

	inst := testInstance(t)
	inst.Listen = ""
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inst.Port = ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	inst.Users["alice"] = User{Password: "pa"}
	inst.Users["bob@x"] = User{Password: "pb", MaxConns: 1}

	m := &Manager{procs: map[int]*managed{}, startErrs: map[int]string{}}
	t.Cleanup(m.StopAll)
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if running, reason := m.Status(inst.Id); !running {
		t.Fatalf("status = %v, %q", running, reason)
	}

	if _, err := connect(t, inst, "alice", "wrong", "ocvt0"); err == nil {
		t.Fatal("a wrong password logged in")
	}
	if _, err := connect(t, inst, "mallory", "pa", "ocvt0"); err == nil {
		t.Fatal("an unknown account logged in")
	}

	alice, err := connect(t, inst, "alice", "pa", "ocvt1")
	if err != nil {
		t.Fatal(err)
	}
	alice.bringUp(t, "10.92.3.1")
	alice.ping(t, "10.92.3.1")
	var up, down int64
	eventually(t, "alice's bytes to be metered", 30*time.Second, func() bool {
		traffic, online := m.CollectTraffic()
		for _, tr := range traffic {
			if tr.Email == "alice" && tr.Tag == inst.Tag {
				up += tr.Up
				down += tr.Down
			}
		}
		return up == pingPackets*pingBytes && down == pingPackets*pingBytes && len(online) == 1 && online[0] == "alice"
	})

	// More traffic, then a kick: what moved since the last poll is still booked, once.
	alice.ping(t, "10.92.3.1")
	if n := m.KickUser("alice"); n != 1 {
		t.Fatalf("KickUser = %d, want 1", n)
	}
	eventually(t, "alice's tunnel to go down", 15*time.Second, func() bool { return !alice.alive() })
	eventually(t, "the final bytes to be booked", 15*time.Second, func() bool {
		traffic, _ := m.CollectTraffic()
		for _, tr := range traffic {
			if tr.Email == "alice" {
				up += tr.Up
				down += tr.Down
			}
		}
		return up == 2*pingPackets*pingBytes && down == 2*pingPackets*pingBytes
	})
	if traffic, _ := m.CollectTraffic(); len(traffic) != 0 {
		t.Fatalf("bytes booked again after the session was settled: %+v", traffic)
	}

	// Bob may hold one session: the newer second one is ended when accounts are re-applied.
	first, err := connect(t, inst, "bob@x", "pb", "ocvt2")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // distinct connect times: the newest is the one to end
	second, err := connect(t, inst, "bob@x", "pb", "ocvt3")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	eventually(t, "bob's second session to end", 15*time.Second, func() bool { return !second.alive() })
	if !first.alive() {
		t.Fatal("bob's first session was ended instead")
	}

	// Dropping an account ends its session and refuses its next login.
	delete(inst.Users, "bob@x")
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	eventually(t, "bob's session to end after he was removed", 15*time.Second, func() bool { return !first.alive() })
	if _, err := connect(t, inst, "bob@x", "pb", "ocvt4"); err == nil {
		t.Fatal("a removed account can still log in")
	}
}
