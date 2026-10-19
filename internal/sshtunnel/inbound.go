package sshtunnel

import (
	"encoding/json"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Settings is an ssh inbound's settings JSON.
type Settings struct {
	HostKey string `json:"hostKey"`
	Banner  string `json:"banner"`
	// AllowLocal lets accounts tunnel to loopback and private addresses.
	AllowLocal bool     `json:"allowLocal"`
	Clients    []Client `json:"clients"`
}

// Client is the part of a client entry the SSH server reads; decoding into it
// drops the rest, so settings are never re-encoded from this type.
type Client struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Enable   bool   `json:"enable"`
	LimitIP  int    `json:"limitIp"`
}

// ServerOf reads what the server needs to listen, whoever the accounts are.
func ServerOf(ib *model.Inbound) (Instance, []Client, bool) {
	if ib == nil || ib.Protocol != model.SSH || ib.Port <= 0 {
		return Instance{}, nil, false
	}
	var s Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil || strings.TrimSpace(s.HostKey) == "" {
		return Instance{}, nil, false
	}
	return Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port,
		HostKey: s.HostKey, Banner: s.Banner, AllowLocal: s.AllowLocal,
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
