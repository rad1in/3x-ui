package openvpn

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

const dataCiphers = "AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305"

// Instance is the desired runtime of one openvpn inbound: one openvpn process.
type Instance struct {
	Id      int
	Tag     string
	Listen  string
	Port    int
	Proto   string // "udp" or "tcp"
	Network netip.Prefix
	DNS     []string
	PKI     PKI
	// Users change in place over the management socket, without a restart.
	Users map[string]User
}

func (inst Instance) device() string {
	return "ovpn" + strconv.Itoa(inst.Id)
}

func (inst Instance) tcp() bool {
	return inst.Proto == "tcp"
}

// structuralFingerprint moves on anything only a restart can apply.
func (inst Instance) structuralFingerprint() string {
	return strings.Join([]string{
		inst.Listen, strconv.Itoa(inst.Port), inst.Proto, inst.Network.String(),
		strings.Join(inst.DNS, ","), inst.PKI.CA, inst.PKI.ServerCert, inst.PKI.ServerKey, inst.PKI.TLSCrypt,
	}, "|")
}

func inline(tag, body string) string {
	return "<" + tag + ">\n" + strings.TrimSpace(body) + "\n</" + tag + ">\n"
}

func renderServerConfig(inst Instance, mgmtSocket string) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	if inst.tcp() {
		line("proto tcp-server")
	} else {
		line("proto udp")
		line("explicit-exit-notify 1")
	}
	if l := strings.TrimSpace(inst.Listen); l != "" && l != "0.0.0.0" && l != "::" {
		line("local %s", l)
	}
	line("port %d", inst.Port)
	line("dev %s", inst.device())
	line("dev-type tun")
	line("topology subnet")
	mask := net.CIDRMask(inst.Network.Bits(), 32)
	line("server %s %s", inst.Network.Masked().Addr(), net.IP(mask).String())
	line("dh none")
	line("ecdh-curve prime256v1")
	line("tls-version-min 1.2")
	line("data-ciphers %s", dataCiphers)
	line("auth SHA256")
	// Clients log in with username/password, checked by the panel over the socket.
	line("verify-client-cert none")
	line("username-as-common-name")
	line("duplicate-cn")
	line("management %s unix", mgmtSocket)
	line("management-client-auth")
	line("reneg-sec 0")
	line("keepalive 10 60")
	line("persist-key")
	line("persist-tun")
	line(`push "redirect-gateway def1 bypass-dhcp"`)
	for _, dns := range inst.DNS {
		line(`push "dhcp-option DNS %s"`, dns)
	}
	line("verb 3")
	b.WriteString(inline("ca", inst.PKI.CA))
	b.WriteString(inline("cert", inst.PKI.ServerCert))
	b.WriteString(inline("key", inst.PKI.ServerKey))
	b.WriteString(inline("tls-crypt", inst.PKI.TLSCrypt))
	return b.String()
}

// ClientProfile is the .ovpn file a user imports; it asks for their username
// and password, so one profile serves every user of the inbound.
func ClientProfile(inst Instance, host string, remark string) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	if remark != "" {
		line("# %s", strings.ReplaceAll(remark, "\n", " "))
	}
	line("client")
	line("dev tun")
	if inst.tcp() {
		line("proto tcp-client")
	} else {
		line("proto udp")
	}
	line("remote %s %d", host, inst.Port)
	line("resolv-retry infinite")
	line("nobind")
	line("persist-key")
	line("persist-tun")
	line("remote-cert-tls server")
	line("auth-user-pass")
	line("auth-nocache")
	line("data-ciphers %s", dataCiphers)
	line("auth SHA256")
	line("verb 3")
	b.WriteString(inline("ca", inst.PKI.CA))
	b.WriteString(inline("tls-crypt", inst.PKI.TLSCrypt))
	return b.String()
}

// usersFingerprint orders users so a reordered client list is not a change.
func usersFingerprint(users map[string]User) string {
	parts := make([]string, 0, len(users))
	for name, u := range users {
		parts = append(parts, name+"="+u.Password+"/"+strconv.Itoa(u.MaxConns))
	}
	slices.Sort(parts)
	return strings.Join(parts, "|")
}
