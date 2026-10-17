package openvpn

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestInstanceFromInboundServesOnlyUsableAccounts(t *testing.T) {
	pki, err := NewPKI("t")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := (Settings{
		Proto: "tcp", Network: "10.88.3.0/24", DNS: []string{"9.9.9.9"}, PKI: pki,
		Clients: []Client{
			{Email: "alice", Password: "a-pass", Enable: true, LimitIP: 2},
			{Email: "off", Password: "x", Enable: false},
			{Email: "nopass", Enable: true},
		},
	}).JSON()
	if err != nil {
		t.Fatal(err)
	}
	inst, ok := InstanceFromInbound(&model.Inbound{Id: 4, Tag: "ovpn-4", Port: 1194, Protocol: model.OpenVPN, Settings: settings})
	if !ok {
		t.Fatal("a configured inbound with one usable account was rejected")
	}
	if len(inst.Users) != 1 || inst.Users["alice"] != (User{Password: "a-pass", MaxConns: 2}) {
		t.Fatalf("users = %+v; want only alice with her IP limit as the connection cap", inst.Users)
	}
	if inst.Proto != "tcp" || inst.Network.String() != "10.88.3.0/24" || inst.DNS[0] != "9.9.9.9" {
		t.Fatalf("instance = %+v", inst)
	}

	bad, _ := (Settings{Proto: "udp", Network: "not-a-net", PKI: pki, Clients: []Client{{Email: "a", Password: "p", Enable: true}}}).JSON()
	if _, ok := InstanceFromInbound(&model.Inbound{Id: 5, Protocol: model.OpenVPN, Settings: bad}); ok {
		t.Fatal("an inbound without a valid client network must not start")
	}
}

func (s Settings) JSON() (string, error) {
	bs, err := json.Marshal(s)
	return string(bs), err
}
