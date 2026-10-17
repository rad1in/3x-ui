package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
)

func savedIKEv2PKI(t *testing.T, ib *model.Inbound) ipsec.PKI {
	t.Helper()
	var s ipsec.Settings
	if err := json.Unmarshal([]byte(ib.Settings), &s); err != nil {
		t.Fatal(err)
	}
	return s.PKI
}

func TestIKEv2SaveFillsCertsPoolPortAndPasswords(t *testing.T) {
	setupConflictDB(t)
	// IKEv2 pools and OpenVPN networks are both routed by the host, so they may not overlap.
	seedInboundConflict(t, "ovpn-taken", "", 1194, model.OpenVPN, "", `{"network":"10.89.0.0/24"}`)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.IKEv2, Port: 4433, Settings: `{"serverAddress":" vpn.example.com ","clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeIKEv2Settings(ib, ""); err != nil {
		t.Fatal(err)
	}
	if ib.Port != 500 {
		t.Errorf("port = %d; IKEv2 clients always dial UDP 500", ib.Port)
	}
	saved := openvpnSettingsOf(t, ib)
	if saved["pool"] != "10.89.1.0/24" || saved["serverAddress"] != "vpn.example.com" {
		t.Errorf("pool = %v, address = %v; want the first free /24 after the OpenVPN one and the trimmed address", saved["pool"], saved["serverAddress"])
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}
	first := savedIKEv2PKI(t, ib)
	inst, _, ok := ipsec.ServerOf(&model.Inbound{Protocol: model.IKEv2, Settings: ib.Settings})
	if !ok || inst.Address != "vpn.example.com" || !first.Complete() {
		t.Fatalf("saved inbound cannot serve: %+v", inst)
	}

	// The form sends the settings back without the certificates; a new address
	// reissues the server cert but keeps the CA every installed profile trusts.
	delete(saved, "pki")
	saved["serverAddress"] = "203.0.113.9"
	edited, _ := json.Marshal(saved)
	again := &model.Inbound{Id: 2, Protocol: model.IKEv2, Port: 500, Settings: string(edited)}
	if err := svc.normalizeIKEv2Settings(again, ib.Settings); err != nil {
		t.Fatal(err)
	}
	moved := savedIKEv2PKI(t, again)
	if moved.CA != first.CA || moved.ServerCert == first.ServerCert {
		t.Fatal("an address change must reissue the server cert under the same CA")
	}

	clash := &model.Inbound{Protocol: model.IKEv2, Settings: `{"serverAddress":"vpn.example.com","pool":"10.89.0.128/25"}`}
	if err := svc.normalizeIKEv2Settings(clash, ""); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("pool overlapping an OpenVPN network: err = %v", err)
	}
}

func TestIKEv2NeedsTheAddressClientsDial(t *testing.T) {
	setupConflictDB(t)
	err := (&InboundService{}).normalizeIKEv2Settings(&model.Inbound{Protocol: model.IKEv2, Settings: `{"serverAddress":"  "}`}, "")
	if err == nil || err.Error() != "ikev2: set the server address (domain or IP) clients connect to" {
		t.Fatalf("err = %v", err)
	}
}

func TestDesiredIKEv2InstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.IKEv2, Settings: `{"serverAddress":"vpn.example.com","clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeIKEv2Settings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ike-1", "", 500, model.IKEv2, "", ib.Settings)
	id := loadInboundByTag(t, "ike-1").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredIKEv2Instances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}

// UDP 500/4500 and the routed pool are host-wide, shared with the owner's panel.
func TestHostedPanelCannotRunIKEv2(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	err := (&InboundService{}).normalizeIKEv2Settings(&model.Inbound{Protocol: model.IKEv2, Settings: `{}`}, "")
	if !errors.Is(err, ErrHostedForbidden) {
		t.Fatalf("err = %v, want ErrHostedForbidden", err)
	}
}
