package service

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
)

func TestSSTPSaveFillsCertificatePoolPortAndPasswords(t *testing.T) {
	setupConflictDB(t)
	// Every host-routed VPN pool counts, IKEv2's included.
	seedInboundConflict(t, "ike-taken", "", 500, model.IKEv2, "", `{"pool":"10.91.0.0/24"}`)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.SSTP, Settings: `{"serverAddress":" vpn.example.com ","clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeSSTPSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	if ib.Port != 443 {
		t.Errorf("port = %d, want the default 443 when none was chosen", ib.Port)
	}
	var saved map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &saved); err != nil {
		t.Fatal(err)
	}
	if saved["pool"] != "10.91.1.0/24" || saved["serverAddress"] != "vpn.example.com" {
		t.Errorf("pool = %v, address = %v; want the first free /24 after IKEv2's and a trimmed address", saved["pool"], saved["serverAddress"])
	}
	inst, _, ok := sstp.ServerOf(&model.Inbound{Protocol: model.SSTP, Port: 443, Settings: ib.Settings})
	if !ok {
		t.Fatal("the saved inbound is not usable")
	}
	ca, err := ipsec.CACert(inst.PKI)
	if err != nil {
		t.Fatal(err)
	}
	server, err := x509.ParseCertificate(mustPEMDER(t, inst.PKI.ServerCert))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.VerifyHostname("vpn.example.com"); err != nil {
		t.Errorf("the server certificate does not name the address: %v", err)
	}
	if err := server.CheckSignatureFrom(ca); err != nil {
		t.Errorf("the server certificate is not signed by the inbound's CA: %v", err)
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}

	// Moving the inbound to another name reissues the server certificate under the same CA,
	// so the CA Windows already trusts keeps working.
	saved["serverAddress"] = "vpn2.example.com"
	edited, _ := json.Marshal(saved)
	again := &model.Inbound{Id: 2, Protocol: model.SSTP, Port: 8443, Settings: string(edited)}
	if err := svc.normalizeSSTPSettings(again, ib.Settings); err != nil {
		t.Fatal(err)
	}
	if again.Port != 8443 {
		t.Errorf("a chosen port was changed to %d", again.Port)
	}
	moved, _, ok := sstp.ServerOf(again)
	if !ok || moved.PKI.CA != inst.PKI.CA || moved.PKI.ServerCert == inst.PKI.ServerCert {
		t.Fatalf("after changing the address: same CA = %v, certificate reissued = %v", moved.PKI.CA == inst.PKI.CA, moved.PKI.ServerCert != inst.PKI.ServerCert)
	}
}

func mustPEMDER(t *testing.T, pemText string) []byte {
	t.Helper()
	cert, err := ipsec.CACert(ipsec.PKI{CA: pemText})
	if err != nil {
		t.Fatal(err)
	}
	return cert.Raw
}

func TestSSTPInboundNeedsAnAddress(t *testing.T) {
	setupConflictDB(t)
	err := (&InboundService{}).normalizeSSTPSettings(&model.Inbound{Protocol: model.SSTP, Settings: `{"serverAddress":"  "}`}, "")
	if err == nil {
		t.Fatal("an SSTP inbound with no address was accepted")
	}
}

func TestDesiredSSTPInstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.SSTP, Settings: `{"serverAddress":"vpn.example.com","clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeSSTPSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "sstp-443", "", 443, model.SSTP, "", ib.Settings)
	id := loadInboundByTag(t, "sstp-443").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredSSTPInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}

func TestSSTPCADownloadIsTheInboundsOwnCA(t *testing.T) {
	setupConflictDB(t)
	ib := &model.Inbound{Protocol: model.SSTP, Settings: `{"serverAddress":"vpn.example.com","clients":[{"email":"a","enable":true}]}`}
	if err := (&InboundService{}).normalizeSSTPSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	ib.Id = 7
	name, mime, body, err := SSTPCA(ib)
	if err != nil {
		t.Fatal(err)
	}
	inst, _, _ := sstp.ServerOf(ib)
	ca, _ := ipsec.CACert(inst.PKI)
	if name != "sstp-7-ca.cer" || mime != "application/pkix-cert" || string(body) != string(ca.Raw) {
		t.Fatalf("download = %q %q, %d bytes; want the inbound's CA in DER", name, mime, len(body))
	}
	if _, _, _, err := SSTPCA(&model.Inbound{Protocol: model.VLESS}); err == nil {
		t.Fatal("a non-SSTP inbound handed out a CA")
	}
}

// The client pool and pppd's secrets file are host-wide.
func TestHostedPanelCannotRunSSTP(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	err := (&InboundService{}).normalizeSSTPSettings(&model.Inbound{Protocol: model.SSTP, Settings: `{"serverAddress":"x"}`}, "")
	if !errors.Is(err, ErrHostedForbidden) {
		t.Fatalf("err = %v, want ErrHostedForbidden", err)
	}
}
