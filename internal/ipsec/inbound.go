package ipsec

import (
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Settings is an ikev2 inbound's settings JSON.
type Settings struct {
	ServerAddress string   `json:"serverAddress"`
	Pool          string   `json:"pool"`
	DNS           []string `json:"dns"`
	PKI           PKI      `json:"pki"`
	Clients       []Client `json:"clients"`
}

// Client is the part of a client entry IKEv2 reads; decoding into it drops
// the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
	SubID    string `json:"subId"`
}

// ServerOf reads what a profile needs (address, certificates), whoever the
// inbound's accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.IKEv2 {
		return Instance{}, nil, false
	}
	var s Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil {
		return Instance{}, nil, false
	}
	pool, err := netip.ParsePrefix(strings.TrimSpace(s.Pool))
	address := strings.TrimSpace(s.ServerAddress)
	if err != nil || !pool.Addr().Is4() || address == "" || !s.PKI.Complete() {
		return Instance{}, nil, false
	}
	return Instance{
		Id: ib.Id, Tag: ib.Tag, Address: address, Pool: pool.Masked(), DNS: s.DNS, PKI: s.PKI,
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
