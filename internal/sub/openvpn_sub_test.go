package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
)

func seedOpenVPNInbound(t *testing.T, subId string) *model.Inbound {
	t.Helper()
	pki, err := openvpn.NewPKI("sub")
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{
		"proto": "tcp", "network": "10.88.0.0/24", "pki": pki,
		"clients": []map[string]any{{"email": "ovpn@e", "password": "pw-123456", "subId": subId, "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "ovpn-1194", Enable: true, Port: 1194, Protocol: model.OpenVPN, Remark: "Tehran OVPN", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "ovpn@e", SubID: subId, Password: "pw-123456", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	return ib
}

// A user who only has OpenVPN still has a subscription: its page lists the
// profile and its login, and the profile downloads with this server's address.
func TestOpenVPNSubscriptionOffersTheProfile(t *testing.T) {
	initSubDB(t)
	ib := seedOpenVPNInbound(t, "OVPNSUB")
	s := &SubService{}
	s.PrepareForRequest("vpn.example.com")

	subs, emails, _, traffic, err := s.getSubs("OVPNSUB")
	if err != nil || subs == nil || len(emails) != len(subs) || !traffic.Enable {
		t.Fatalf("getSubs = %v, %v, enabled %v, %v; an OpenVPN-only user must still have a subscription", subs, emails, traffic.Enable, err)
	}
	for _, link := range subs {
		if strings.TrimSpace(link) == "" {
			t.Fatal("an OpenVPN inbound added an empty share link")
		}
	}
	want := OpenVPNProfile{InboundId: ib.Id, Remark: "Tehran OVPN", Username: "ovpn@e", Password: "pw-123456"}
	if len(s.openvpnProfiles) != 1 || s.openvpnProfiles[0] != want {
		t.Fatalf("profiles = %+v, want %+v", s.openvpnProfiles, want)
	}

	name, body, err := s.OpenVPNProfileFile("OVPNSUB", ib.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"remote vpn.example.com 1194", "proto tcp-client", "auth-user-pass", "<ca>", "<tls-crypt>"} {
		if !strings.Contains(body, line) {
			t.Errorf("profile lacks %q:\n%s", line, body)
		}
	}
	if name != "Tehran-OVPN.ovpn" {
		t.Errorf("file name = %q", name)
	}
	if _, _, err := s.OpenVPNProfileFile("SOMEONE-ELSE", ib.Id); err == nil {
		t.Fatal("a subscription id that is not on the inbound downloaded its profile")
	}
}
