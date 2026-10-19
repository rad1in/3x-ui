package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
)

func TestOCServSaveFillsCertificatePoolPortAndPasswords(t *testing.T) {
	setupConflictDB(t)
	// Every host-routed VPN pool counts, SSTP's included.
	seedInboundConflict(t, "sstp-taken", "", 443, model.SSTP, "", `{"pool":"10.92.0.0/24"}`)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.OCServ, Settings: `{"serverAddress":" vpn.example.com ","clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeOCServSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	if ib.Port != 443 {
		t.Errorf("port = %d, want the default 443 when none was chosen", ib.Port)
	}
	var saved map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &saved); err != nil {
		t.Fatal(err)
	}
	if saved["pool"] != "10.92.1.0/24" || saved["serverAddress"] != "vpn.example.com" {
		t.Errorf("pool = %v, address = %v; want the first free /24 after SSTP's and a trimmed address", saved["pool"], saved["serverAddress"])
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}
	inst, _, ok := ocserv.ServerOf(&model.Inbound{Protocol: model.OCServ, Port: 443, Settings: ib.Settings})
	if !ok {
		t.Fatal("the saved inbound is not usable")
	}

	// A new address reissues the server certificate under the same CA, which clients already trust.
	saved["serverAddress"] = "vpn2.example.com"
	edited, _ := json.Marshal(saved)
	again := &model.Inbound{Id: 2, Protocol: model.OCServ, Port: 8443, Settings: string(edited)}
	if err := svc.normalizeOCServSettings(again, ib.Settings); err != nil {
		t.Fatal(err)
	}
	moved, _, ok := ocserv.ServerOf(again)
	if !ok || again.Port != 8443 || moved.PKI.CA != inst.PKI.CA || moved.PKI.ServerCert == inst.PKI.ServerCert {
		t.Fatalf("after changing the address: port %d, same CA = %v, certificate reissued = %v", again.Port, moved.PKI.CA == inst.PKI.CA, moved.PKI.ServerCert != inst.PKI.ServerCert)
	}
}

func TestOCServInboundNeedsAnAddress(t *testing.T) {
	setupConflictDB(t)
	if err := (&InboundService{}).normalizeOCServSettings(&model.Inbound{Protocol: model.OCServ, Settings: `{"serverAddress":" "}`}, ""); err == nil {
		t.Fatal("an ocserv inbound with no address was accepted")
	}
}

func TestDesiredOCServInstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.OCServ, Settings: `{"serverAddress":"vpn.example.com","clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeOCServSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ocserv-443", "", 443, model.OCServ, "", ib.Settings)
	id := loadInboundByTag(t, "ocserv-443").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredOCServInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}

func TestOCServCADownloadIsTheInboundsOwnCA(t *testing.T) {
	setupConflictDB(t)
	ib := &model.Inbound{Protocol: model.OCServ, Settings: `{"serverAddress":"vpn.example.com","clients":[{"email":"a","enable":true}]}`}
	if err := (&InboundService{}).normalizeOCServSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	ib.Id = 7
	name, mime, body, err := OCServCA(ib)
	if err != nil {
		t.Fatal(err)
	}
	inst, _, _ := ocserv.ServerOf(ib)
	ca, _ := ipsec.CACert(inst.PKI)
	if name != "ocserv-7-ca.cer" || mime != "application/pkix-cert" || string(body) != string(ca.Raw) {
		t.Fatalf("download = %q %q, %d bytes; want the inbound's CA in DER", name, mime, len(body))
	}
	if _, _, _, err := OCServCA(&model.Inbound{Protocol: model.VLESS}); err == nil {
		t.Fatal("a non-ocserv inbound handed out a CA")
	}
}

// Tunnel devices and client networks are host-wide.
func TestHostedPanelCannotRunOCServ(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	err := (&InboundService{}).normalizeOCServSettings(&model.Inbound{Protocol: model.OCServ, Settings: `{"serverAddress":"x"}`}, "")
	if !errors.Is(err, ErrHostedForbidden) {
		t.Fatalf("err = %v, want ErrHostedForbidden", err)
	}
}
