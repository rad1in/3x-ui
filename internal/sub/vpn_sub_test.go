package sub

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
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
	want := VPNAccount{InboundId: ib.Id, Protocol: "openvpn", Remark: "Tehran OVPN", Server: "vpn.example.com", Username: "ovpn@e", Password: "pw-123456"}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
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

func seedIKEv2Inbound(t *testing.T, subId string) *model.Inbound {
	t.Helper()
	pki, err := ipsec.PKIFor(ipsec.PKI{}, "ike.example.com")
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{
		"serverAddress": "ike.example.com", "pool": "10.89.0.0/24", "pki": pki,
		"clients": []map[string]any{{"email": "ike@e", "password": "pw-ike-1", "subId": subId, "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "ike-500", Enable: true, Port: 500, Protocol: model.IKEv2, Remark: "Phones", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "ike@e", SubID: subId, Password: "pw-ike-1", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	return ib
}

// An IKEv2 account is listed with the address its certificate names, which is
// what native clients must dial, not whatever host served the page.
func TestIKEv2SubscriptionOffersItsProfiles(t *testing.T) {
	initSubDB(t)
	ib := seedIKEv2Inbound(t, "IKESUB")
	s := &SubService{}
	s.PrepareForRequest("panel.example.net")

	subs, _, _, traffic, err := s.getSubs("IKESUB")
	if err != nil || subs == nil || !traffic.Enable {
		t.Fatalf("getSubs = %v, enabled %v, %v; an IKEv2-only user must still have a subscription", subs, traffic.Enable, err)
	}
	want := VPNAccount{InboundId: ib.Id, Protocol: "ikev2", Remark: "Phones", Server: "ike.example.com", Username: "ike@e", Password: "pw-ike-1"}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
	}

	name, _, body, err := s.IKEv2ProfileFile("IKESUB", ib.Id, "mobileconfig")
	if err != nil || name != "ikev2-"+strconv.Itoa(ib.Id)+".mobileconfig" {
		t.Fatalf("mobileconfig %q: %v", name, err)
	}
	if !strings.Contains(string(body), "<string>pw-ike-1</string>") {
		t.Fatal("the Apple profile does not carry the login's password")
	}
	if _, _, _, err := s.IKEv2ProfileFile("SOMEONE-ELSE", ib.Id, "mobileconfig"); err == nil {
		t.Fatal("a subscription id that is not on the inbound downloaded its profile")
	}
}

// L2TP has no profile file: the page lists everything a phone's built-in
// client asks for, the pre-shared key included.
func TestL2TPSubscriptionListsTheKeyAndLogin(t *testing.T) {
	initSubDB(t)
	settings, _ := json.Marshal(map[string]any{
		"psk": "shared-key-123", "pool": "10.90.0.0/24",
		"clients": []map[string]any{{"email": "l2@e", "password": "pw-l2", "subId": "L2SUB", "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "l2tp-1701", Enable: true, Port: 1701, Protocol: model.L2TP, Remark: "Old phones", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "l2@e", SubID: "L2SUB", Password: "pw-l2", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	s := &SubService{}
	s.PrepareForRequest("vpn.example.com")
	if subs, _, _, _, err := s.getSubs("L2SUB"); err != nil || subs == nil {
		t.Fatalf("getSubs = %v, %v; an L2TP-only user must still have a subscription", subs, err)
	}
	want := VPNAccount{InboundId: ib.Id, Protocol: "l2tp", Remark: "Old phones", Server: "vpn.example.com", Username: "l2@e", Password: "pw-l2", PSK: "shared-key-123"}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
	}
}

// An SSH tunnel app asks for the server, port, login and, to trust the
// server, its key fingerprint; the page lists all of them.
func TestSSHSubscriptionListsPortLoginAndFingerprint(t *testing.T) {
	initSubDB(t)
	key, err := sshtunnel.NewHostKey()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{
		"hostKey": key,
		"clients": []map[string]any{{"email": "s@e", "password": "pw-ssh", "subId": "SSHSUB", "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "ssh-2222", Enable: true, Port: 2222, Protocol: model.SSH, Remark: "Tunnel", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "s@e", SubID: "SSHSUB", Password: "pw-ssh", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	s := &SubService{}
	s.PrepareForRequest("vpn.example.com")
	if subs, _, _, _, err := s.getSubs("SSHSUB"); err != nil || subs == nil {
		t.Fatalf("getSubs = %v, %v; an SSH-only user must still have a subscription", subs, err)
	}
	want := VPNAccount{
		InboundId: ib.Id, Protocol: "ssh", Remark: "Tunnel", Server: "vpn.example.com", Username: "s@e", Password: "pw-ssh",
		Port: 2222, Fingerprint: sshtunnel.Fingerprint(key),
	}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
	}
}

// An SSTP login is the server's name, port and account; the CA to trust comes
// from the same subscription, and only for a subscription that has an account there.
func TestSSTPSubscriptionListsTheAccountAndGatesItsCA(t *testing.T) {
	initSubDB(t)
	pki, err := ipsec.PKIFor(ipsec.PKI{}, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{
		"serverAddress": "vpn.example.com", "pool": "10.91.0.0/24", "pki": pki,
		"clients": []map[string]any{{"email": "w@e", "password": "pw-sstp", "subId": "SSTPSUB", "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "sstp-443", Enable: true, Port: 443, Protocol: model.SSTP, Remark: "Windows", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "w@e", SubID: "SSTPSUB", Password: "pw-sstp", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	s := &SubService{}
	s.PrepareForRequest("panel.example.net")
	if subs, _, _, _, err := s.getSubs("SSTPSUB"); err != nil || subs == nil {
		t.Fatalf("getSubs = %v, %v; an SSTP-only user must still have a subscription", subs, err)
	}
	// The server is the certificate's name, not the host the panel was reached on.
	want := VPNAccount{InboundId: ib.Id, Protocol: "sstp", Remark: "Windows", Server: "vpn.example.com", Username: "w@e", Password: "pw-sstp", Port: 443}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
	}

	name, mime, body, err := (&SubService{}).SSTPCAFile("SSTPSUB", ib.Id)
	inst, _, _ := sstp.ServerOf(ib)
	ca, _ := ipsec.CACert(inst.PKI)
	if err != nil || name != "sstp-"+strconv.Itoa(ib.Id)+"-ca.cer" || mime != "application/pkix-cert" || string(body) != string(ca.Raw) {
		t.Fatalf("CA download = %q %q %d bytes, %v", name, mime, len(body), err)
	}
	if _, _, _, err := (&SubService{}).SSTPCAFile("SOMEONE-ELSE", ib.Id); err == nil {
		t.Fatal("a subscription with no account on the inbound downloaded its CA")
	}
}

// An OpenConnect login is the server's name, port and account, with the CA to
// trust from the same subscription and only for one that has an account there.
func TestOCServSubscriptionListsTheAccountAndGatesItsCA(t *testing.T) {
	initSubDB(t)
	pki, err := ipsec.PKIFor(ipsec.PKI{}, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{
		"serverAddress": "vpn.example.com", "pool": "10.92.0.0/24", "pki": pki,
		"clients": []map[string]any{{"email": "o@e", "password": "pw-oc", "subId": "OCSUB", "enable": true}},
	})
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: "ocserv-8443", Enable: true, Port: 8443, Protocol: model.OCServ, Remark: "AnyConnect", Settings: string(settings)}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "o@e", SubID: "OCSUB", Password: "pw-oc", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	s := &SubService{}
	s.PrepareForRequest("panel.example.net")
	if subs, _, _, _, err := s.getSubs("OCSUB"); err != nil || subs == nil {
		t.Fatalf("getSubs = %v, %v; an ocserv-only user must still have a subscription", subs, err)
	}
	want := VPNAccount{InboundId: ib.Id, Protocol: "ocserv", Remark: "AnyConnect", Server: "vpn.example.com", Username: "o@e", Password: "pw-oc", Port: 8443}
	if len(s.vpnAccounts) != 1 || s.vpnAccounts[0] != want {
		t.Fatalf("accounts = %+v, want %+v", s.vpnAccounts, want)
	}

	name, mime, body, err := (&SubService{}).OCServCAFile("OCSUB", ib.Id)
	inst, _, _ := ocserv.ServerOf(ib)
	ca, _ := ipsec.CACert(inst.PKI)
	if err != nil || name != "ocserv-"+strconv.Itoa(ib.Id)+"-ca.cer" || mime != "application/pkix-cert" || string(body) != string(ca.Raw) {
		t.Fatalf("CA download = %q %q %d bytes, %v", name, mime, len(body), err)
	}
	if _, _, _, err := (&SubService{}).OCServCAFile("SOMEONE-ELSE", ib.Id); err == nil {
		t.Fatal("a subscription with no account on the inbound downloaded its CA")
	}
}
