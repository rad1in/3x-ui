package openvpn

import (
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Settings is an openvpn inbound's settings JSON.
type Settings struct {
	Proto   string   `json:"proto"`
	Network string   `json:"network"`
	DNS     []string `json:"dns"`
	PKI     PKI      `json:"pki"`
	Clients []Client `json:"clients"`
}

// Client is the part of a client entry openvpn reads; decoding into it drops
// the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
}

// ServerOf reads what a profile needs (address, transport, certificates),
// whoever the inbound's accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.OpenVPN {
		return Instance{}, nil, false
	}
	var s Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil {
		return Instance{}, nil, false
	}
	network, err := netip.ParsePrefix(strings.TrimSpace(s.Network))
	if err != nil || !network.Addr().Is4() || !s.PKI.Complete() {
		return Instance{}, nil, false
	}
	proto := "udp"
	if s.Proto == "tcp" {
		proto = "tcp"
	}
	return Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port, Proto: proto,
		Network: network.Masked(), DNS: s.DNS, PKI: s.PKI,
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
