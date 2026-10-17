package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
)

func openvpnSettingsOf(t *testing.T, ib *model.Inbound) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOpenVPNSaveFillsCertsNetworkAndPasswords(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "ovpn-taken", "", 1194, model.OpenVPN, "", `{"network":"10.88.0.0/24"}`)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.OpenVPN, Settings: `{"clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeOpenVPNSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	saved := openvpnSettingsOf(t, ib)
	if saved["network"] != "10.88.1.0/24" {
		t.Errorf("network = %v; the first free /24 after the taken 10.88.0.0/24", saved["network"])
	}
	pki, _ := saved["pki"].(map[string]any)
	if !strings.Contains(pki["ca"].(string), "BEGIN CERTIFICATE") || !strings.Contains(pki["tlsCrypt"].(string), "OpenVPN Static key V1") {
		t.Fatalf("pki not generated: %v", pki)
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}

	// The form sends the settings back without the certificates; they must survive.
	delete(saved, "pki")
	saved["dns"] = []any{"9.9.9.9"}
	edited, _ := json.Marshal(saved)
	again := &model.Inbound{Id: 2, Protocol: model.OpenVPN, Settings: string(edited)}
	if err := svc.normalizeOpenVPNSettings(again, ib.Settings); err != nil {
		t.Fatal(err)
	}
	if openvpnSettingsOf(t, again)["pki"].(map[string]any)["ca"] != pki["ca"] {
		t.Fatal("an edit regenerated the CA, breaking every profile already handed out")
	}

	clash := &model.Inbound{Protocol: model.OpenVPN, Settings: `{"network":"10.88.0.128/25"}`}
	if err := svc.normalizeOpenVPNSettings(clash, ""); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("overlapping client network: err = %v", err)
	}
}

func TestDesiredOpenVPNInstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.OpenVPN, Settings: `{"clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeOpenVPNSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ovpn-1", "", 1194, model.OpenVPN, "", ib.Settings)
	id := loadInboundByTag(t, "ovpn-1").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredOpenVPNInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}

// Tunnel device names and client networks are host-wide; a hosted panel's
// openvpn would collide with the owner's, so it is refused there.
func TestHostedPanelCannotRunOpenVPN(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	err := (&InboundService{}).normalizeOpenVPNSettings(&model.Inbound{Protocol: model.OpenVPN, Settings: `{}`}, "")
	if !errors.Is(err, ErrHostedForbidden) {
		t.Fatalf("err = %v, want ErrHostedForbidden", err)
	}
}

// A client added through the clients page must reach openvpn with a password,
// whether the admin typed one or left it to be generated.
func TestOpenVPNClientCreateReachesTheServerWithAPassword(t *testing.T) {
	setupConflictDB(t)
	inbounds := &InboundService{}
	ib := &model.Inbound{Protocol: model.OpenVPN, Settings: `{}`}
	if err := inbounds.normalizeOpenVPNSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ovpn-c", "", 1194, model.OpenVPN, "", ib.Settings)
	id := loadInboundByTag(t, "ovpn-c").Id

	clients := &ClientService{}
	for _, c := range []model.Client{{Email: "bob", Password: "bob-secret", Enable: true}, {Email: "eve", Enable: true}} {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: c, InboundIds: []int{id}}); err != nil {
			t.Fatalf("create %s: %v", c.Email, err)
		}
	}
	inst, ok := openvpn.InstanceFromInbound(loadInboundByTag(t, "ovpn-c"))
	if !ok || inst.Users["bob"].Password != "bob-secret" || len(inst.Users["eve"].Password) < 8 {
		t.Fatalf("users = %+v; bob keeps his password, eve gets a generated one", inst.Users)
	}
}
