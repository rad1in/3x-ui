package ocserv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
)

func testPKI(t *testing.T) ipsec.PKI {
	t.Helper()
	pki, err := ipsec.PKIFor(ipsec.PKI{}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return pki
}

func testInstance(t *testing.T) Instance {
	t.Helper()
	ib := &model.Inbound{Id: 5, Protocol: model.OCServ, Port: 24444, Tag: "ocserv-24444", Listen: "127.0.0.1"}
	settings := `{"serverAddress":"127.0.0.1","pool":"10.92.3.0/24","dns":["1.1.1.1","9.9.9.9"],"pki":` + pkiJSON(t) + `,"clients":[` +
		`{"email":"alice","password":"pa","enable":true},{"email":"bob@x","password":"pb","enable":true,"limitIp":1},` +
		`{"email":"off","password":"p","enable":false},{"email":"has:colon","password":"p","enable":true},{"email":"sp ace","password":"p","enable":true}]}`
	ib.Settings = settings
	inst, ok := InstanceFromInbound(ib)
	if !ok {
		t.Fatal("the test inbound is not usable")
	}
	return inst
}

func TestOnlyAccountsThePasswdFileCanHoldAreServed(t *testing.T) {
	inst := testInstance(t)
	if len(inst.Users) != 2 || inst.Users["alice"].Password != "pa" || inst.Users["bob@x"] != (User{Password: "pb", MaxConns: 1}) {
		t.Fatalf("users = %+v; want alice and bob only, not the disabled or unrepresentable names", inst.Users)
	}
}

func TestPasswdFileListsHashedAccounts(t *testing.T) {
	got, err := passwdFile(map[string]User{"bob@x": {Password: "pb"}, "alice": {Password: "pa"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "alice:*:$6$") || !strings.HasPrefix(lines[1], "bob@x:*:$6$") {
		t.Fatalf("passwd =\n%s", got)
	}
	if strings.Contains(got, "pa\n") || strings.Contains(got, "pb\n") {
		t.Fatal("a password is stored in the clear")
	}
}

func TestRenderedConfigNamesTheInboundsOwnPortsFilesAndNetwork(t *testing.T) {
	inst := testInstance(t)
	p := pathsFor(inst.Id, "/var/x/ocserv.d", "/run/eternal-ocserv")
	got := renderConfig(inst, p, "nobody", "nogroup")
	for _, want := range []string{
		"tcp-port = 24444", "udp-port = 24444", "listen-host = 127.0.0.1",
		`auth = "plain[passwd=/var/x/ocserv.d/5/passwd]"`,
		"socket-file = /run/eternal-ocserv/oc5", "occtl-socket-file = /var/x/ocserv.d/5/occtl.sock",
		"ipv4-network = 10.92.3.0", "ipv4-netmask = 255.255.255.0", "dns = 1.1.1.1", "dns = 9.9.9.9",
		"device = ocv5", "disconnect-script = /var/x/ocserv.d/5/disconnect.sh", "stats-report-time = 5",
		"max-same-clients = 0",
	} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("config lacks %q:\n%s", want, got)
		}
	}
}

// ocserv itself is the judge of its configuration: -t loads it without starting.
func TestRealOcservAcceptsTheRenderedConfig(t *testing.T) {
	bin, err := exec.LookPath("ocserv")
	if err != nil {
		t.Skip("ocserv is not installed")
	}
	inst := testInstance(t)
	dir := t.TempDir()
	p := pathsFor(inst.Id, dir, dir)
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	passwd, _ := passwdFile(inst.Users)
	for path, body := range map[string]string{
		p.cert: inst.PKI.ServerCert, p.key: inst.PKI.ServerKey, p.passwd: passwd,
		p.conf: renderConfig(inst, p, "nobody", "nogroup"),
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command(bin, "-t", "-c", p.conf).CombinedOutput(); err != nil {
		t.Fatalf("ocserv -t rejected the config: %v\n%s\n%s", err, out, filepath.Base(p.conf))
	}
	// And it must reject a config that is wrong, or the check above proves nothing.
	if err := os.WriteFile(p.conf, []byte("tcp-port = notaport\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(bin, "-t", "-c", p.conf).Run(); err == nil {
		t.Fatal("ocserv -t accepted a nonsense config; the check cannot fail")
	}
}
