package l2tp

import (
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Settings is an l2tp inbound's settings JSON.
type Settings struct {
	PSK     string   `json:"psk"`
	Pool    string   `json:"pool"`
	DNS     []string `json:"dns"`
	Clients []Client `json:"clients"`
}

// Client is the part of a client entry L2TP reads; decoding into it drops
// the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
}

// ServerOf reads what a client needs to connect (pool, PSK), whoever the
// inbound's accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.L2TP {
		return Instance{}, nil, false
	}
	var s Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil {
		return Instance{}, nil, false
	}
	pool, err := netip.ParsePrefix(strings.TrimSpace(s.Pool))
	if err != nil || !pool.Addr().Is4() || pool.Bits() > 29 || strings.TrimSpace(s.PSK) == "" {
		return Instance{}, nil, false
	}
	return Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Pool: pool.Masked(), DNS: s.DNS, PSK: s.PSK,
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
