package ocserv

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
)

// Settings is an ocserv inbound's settings JSON.
type Settings struct {
	ServerAddress string    `json:"serverAddress"`
	Pool          string    `json:"pool"`
	DNS           []string  `json:"dns"`
	PKI           ipsec.PKI `json:"pki"`
	Clients       []Client  `json:"clients"`
}

// Client is the part of a client entry ocserv reads; decoding into it drops
// the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
}

// User is one account an ocserv inbound lets in.
type User struct {
	Password string
	MaxConns int
}

// Instance is the desired state of one ocserv inbound.
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

// structuralFingerprint moves on anything only an ocserv restart applies.
func (i Instance) structuralFingerprint() string {
	return strings.Join([]string{
		i.Listen, strconv.Itoa(i.Port), i.Address, i.Pool.String(), strings.Join(i.DNS, ","),
		i.PKI.ServerCert, i.PKI.ServerKey,
	}, "|")
}

// ServerOf reads what a client needs to connect (address, certificates),
// whoever the inbound's accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.OCServ || ib.Port <= 0 {
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
		if c.Enable && listable(c.Email) && c.Password != "" {
			inst.Users[c.Email] = User{Password: c.Password, MaxConns: max(c.LimitIP, 0)}
		}
	}
	if len(inst.Users) == 0 {
		return Instance{}, false
	}
	return inst, true
}

// listable is whether a name fits ocserv's passwd file, where ':' separates fields.
func listable(name string) bool {
	return name != "" && !strings.ContainsAny(name, ": \r\n\t")
}
