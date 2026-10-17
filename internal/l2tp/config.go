package l2tp

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
)

// User is one account an L2TP inbound lets in with MS-CHAPv2.
type User struct {
	Password string
	MaxConns int
}

// Instance is the desired state of the L2TP inbound: xl2tpd owns UDP 1701 on
// the host, so a server runs one, its IPsec side on the shared charon.
type Instance struct {
	Id     int
	Tag    string
	Listen string
	Pool   netip.Prefix
	DNS    []string
	PSK    string
	Users  map[string]User
}

// poolRange gives the server the pool's first host and clients the rest.
func poolRange(pool netip.Prefix) (local, first, last netip.Addr) {
	p := pool.Masked()
	base := binary.BigEndian.Uint32(p.Addr().AsSlice())
	size := uint32(1) << (32 - p.Bits())
	at := func(n uint32) netip.Addr {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], base+n)
		return netip.AddrFrom4(b)
	}
	return at(1), at(2), at(size - 2)
}

// structuralFingerprint moves on anything only an xl2tpd restart applies.
func (inst Instance) structuralFingerprint() string {
	return strings.Join([]string{inst.Listen, inst.Pool.String(), strings.Join(inst.DNS, ",")}, "|")
}

func renderXL2TPD(inst Instance, dir string) string {
	local, first, last := poolRange(inst.Pool)
	var b strings.Builder
	b.WriteString("[global]\nport = 1701\n")
	if l := strings.TrimSpace(inst.Listen); l != "" && l != "0.0.0.0" && l != "::" {
		fmt.Fprintf(&b, "listen-addr = %s\n", l)
	}
	fmt.Fprintf(&b, `
[lns default]
ip range = %s-%s
local ip = %s
require chap = yes
refuse pap = yes
require authentication = yes
name = x-ui
pppoptfile = %s
length bit = yes
`, first, last, local, filepath.Join(dir, "options.xl2tpd"))
	return b.String()
}

func renderPPPOptions(inst Instance, dir string) string {
	var b strings.Builder
	for _, line := range []string{
		"require-mschap-v2", "refuse-pap", "refuse-chap", "refuse-mschap",
		"ipcp-accept-local", "ipcp-accept-remote", "noccp", "auth",
		"mtu 1280", "mru 1280", "proxyarp", "nodefaultroute",
		"lcp-echo-failure 4", "lcp-echo-interval 30", "connect-delay 5000",
	} {
		b.WriteString(line + "\n")
	}
	for _, d := range inst.DNS {
		fmt.Fprintf(&b, "ms-dns %s\n", d)
	}
	fmt.Fprintf(&b, "ip-up-script %s\nip-down-script %s\n", filepath.Join(dir, "ip-up"), filepath.Join(dir, "ip-down"))
	return b.String()
}

// The scripts record each link for the panel: who, which pppd, and on the way
// down the totals pppd counted; ip-up passes the interface as $1, the peer as $5.
func renderIPUp(dir string) string {
	s := filepath.Join(dir, "sessions")
	return fmt.Sprintf(`#!/bin/sh
mkdir -p '%[1]s'
printf '%%s %%s %%s\n' "$PPPD_PID" "$5" "$PEERNAME" > '%[1]s'/"$1.tmp" && mv '%[1]s'/"$1.tmp" '%[1]s'/"$1"
`, s)
}

func renderIPDown(dir string) string {
	s, e := filepath.Join(dir, "sessions"), filepath.Join(dir, "ended")
	return fmt.Sprintf(`#!/bin/sh
mkdir -p '%[2]s'
printf '%%s %%s %%s\n' "${BYTES_RCVD:-0}" "${BYTES_SENT:-0}" "$PEERNAME" > '%[2]s'/"$1.$PPPD_PID.tmp" && mv '%[2]s'/"$1.$PPPD_PID.tmp" '%[2]s'/"$1.$PPPD_PID"
rm -f '%[1]s'/"$1"
`, s, e)
}
