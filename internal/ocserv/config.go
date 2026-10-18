package ocserv

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// paths are the files one ocserv inbound owns.
type paths struct {
	dir      string
	conf     string
	passwd   string
	cert     string
	key      string
	pid      string
	occtl    string
	discon   string
	ended    string
	sockBase string
}

func pathsFor(id int, runDir, sockDir string) paths {
	dir := filepath.Join(runDir, strconv.Itoa(id))
	return paths{
		dir: dir, conf: filepath.Join(dir, "ocserv.conf"), passwd: filepath.Join(dir, "passwd"),
		cert: filepath.Join(dir, "server.pem"), key: filepath.Join(dir, "server.key"),
		pid: filepath.Join(dir, "ocserv.pid"), occtl: filepath.Join(dir, "occtl.sock"),
		discon: filepath.Join(dir, "disconnect.sh"),
		ended:  filepath.Join(dir, "ended"), sockBase: filepath.Join(sockDir, "oc"+strconv.Itoa(id)),
	}
}

// socketDir holds the sockets ocserv's unprivileged workers connect to, so
// every directory up to it must be traversable by them.
func socketDir() (string, error) {
	for _, base := range []string{"/run", "/var/run", os.TempDir()} {
		if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
			continue
		}
		dir := filepath.Join(base, "x-ui-ocserv")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			continue
		}
		return dir, nil
	}
	return "", fmt.Errorf("ocserv: no directory for its sockets")
}

// dropTo names the unprivileged account ocserv's workers run as.
func dropTo() (usr, grp string) {
	usr = "nobody"
	for _, g := range []string{"nogroup", "nobody", "daemon"} {
		if _, err := user.LookupGroup(g); err == nil {
			return usr, g
		}
	}
	return usr, "nobody"
}

func netmask(p netip.Prefix) string {
	return net.IP(net.CIDRMask(p.Bits(), 32)).String()
}

func renderConfig(inst Instance, p paths, runAsUser, runAsGroup string) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line(`auth = "plain[passwd=%s]"`, p.passwd)
	line("tcp-port = %d", inst.Port)
	line("udp-port = %d", inst.Port)
	if l := strings.TrimSpace(inst.Listen); l != "" && l != "0.0.0.0" && l != "::" {
		line("listen-host = %s", l)
	}
	line("run-as-user = %s", runAsUser)
	line("run-as-group = %s", runAsGroup)
	line("socket-file = %s", p.sockBase)
	line("occtl-socket-file = %s", p.occtl)
	line("pid-file = %s", p.pid)
	line("server-cert = %s", p.cert)
	line("server-key = %s", p.key)
	line("use-occtl = true")
	line("isolate-workers = false")
	line("max-clients = 0")
	line("max-same-clients = 0")
	// Workers report byte counts to the main process this often; the panel meters from them.
	line("stats-report-time = 5")
	line("keepalive = 32400")
	line("dpd = 90")
	line("mobile-dpd = 1800")
	line("try-mtu-discovery = false")
	line("predictable-ips = false")
	line("cisco-client-compat = true")
	line("dtls-legacy = true")
	line("device = ocv%d", inst.Id)
	line("ipv4-network = %s", inst.Pool.Addr())
	line("ipv4-netmask = %s", netmask(inst.Pool))
	for _, d := range inst.DNS {
		line("dns = %s", d)
	}
	line("tunnel-all-dns = true")
	line("route = default")
	line("disconnect-script = %s", p.discon)
	return b.String()
}

// The scripts leave a file per session for the panel: the final byte counts
// when it ends, so the last seconds of a session are booked too.
func renderDisconnectScript(p paths) string {
	return fmt.Sprintf(`#!/bin/sh
mkdir -p '%[1]s'
printf '%%s %%s %%s\n' "${STATS_BYTES_IN:-0}" "${STATS_BYTES_OUT:-0}" "$USERNAME" > '%[1]s'/"$ID.tmp" && mv '%[1]s'/"$ID.tmp" '%[1]s'/"$ID"
`, p.ended)
}

// passwdFile lists the accounts in ocpasswd's format, one crypt hash each.
func passwdFile(users map[string]User) (string, error) {
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		hash, err := hashPassword(users[name].Password)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s:*:%s\n", name, hash)
	}
	return b.String(), nil
}
