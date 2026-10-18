package sstp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
)

// Settings is an sstp inbound's settings JSON.
type Settings struct {
	ServerAddress string    `json:"serverAddress"`
	Pool          string    `json:"pool"`
	DNS           []string  `json:"dns"`
	PKI           ipsec.PKI `json:"pki"`
	Clients       []Client  `json:"clients"`
}

// Client is the part of a client entry SSTP reads; decoding into it drops
// the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
}

// User is one account an SSTP inbound lets in with MS-CHAPv2.
type User struct {
	Password string
	MaxConns int
}

// Instance is the desired state of one sstp inbound.
type Instance struct {
	Id      int
	Tag     string
	Listen  string
	Port    int
	Address string
	Pool    netip.Prefix
	DNS     []string
	PKI     ipsec.PKI
	Users   map[string]User
}

// pppName is how this inbound's pppd knows itself, so the logins written for
// it in chap-secrets are not good for another inbound's pppd.
func (i Instance) pppName() string { return "x-ui-sstp-" + strconv.Itoa(i.Id) }

// structuralFingerprint moves on anything only a rebuilt listener applies.
func (i Instance) structuralFingerprint() string {
	return strings.Join([]string{
		i.Listen, strconv.Itoa(i.Port), i.Address, i.Pool.String(), strings.Join(i.DNS, ","),
		i.PKI.ServerCert, i.PKI.ServerKey,
	}, "|")
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

// ServerOf reads what a client needs to connect (address, pool, certificates),
// whoever the inbound's accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.SSTP || ib.Port <= 0 {
		return Instance{}, nil, false
	}
	var s Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil {
		return Instance{}, nil, false
	}
	pool, err := netip.ParsePrefix(strings.TrimSpace(s.Pool))
	address := strings.TrimSpace(s.ServerAddress)
	if err != nil || !pool.Addr().Is4() || pool.Bits() > 29 || address == "" || !s.PKI.Complete() {
		return Instance{}, nil, false
	}
	return Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port, Address: address,
		Pool: pool.Masked(), DNS: s.DNS, PKI: s.PKI,
	}, s.Clients, true
}

// InstanceFromInbound returns false for an inbound that cannot serve anyone.
func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	inst, clients, ok := ServerOf(ib)
	if !ok {
		return Instance{}, false
	}
	inst.Users = map[string]User{}
	for _, c := range clients {
		if c.Enable && c.Email != "" && c.Password != "" {
			inst.Users[c.Email] = User{Password: c.Password, MaxConns: max(c.LimitIP, 0)}
		}
	}
	if len(inst.Users) == 0 {
		return Instance{}, false
	}
	return inst, true
}

func (i Instance) pppOptions() string {
	var b strings.Builder
	for _, line := range []string{
		"require-mschap-v2", "refuse-pap", "refuse-chap", "refuse-mschap", "refuse-eap",
		"noccp", "auth", "nodefaultroute", "proxyarp", "noaccomp", "nopcomp", "default-asyncmap",
		"mtu 1400", "mru 1400", "lcp-echo-failure 4", "lcp-echo-interval 30",
	} {
		b.WriteString(line + "\n")
	}
	for _, d := range i.DNS {
		fmt.Fprintf(&b, "ms-dns %s\n", d)
	}
	return b.String()
}
