package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
)

func TestL2TPSaveFillsPSKPoolPortAndPasswords(t *testing.T) {
	setupConflictDB(t)
	// Every host-routed VPN pool counts, IKEv2's included.
	seedInboundConflict(t, "ike-taken", "", 500, model.IKEv2, "", `{"pool":"10.90.0.0/24"}`)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.L2TP, Port: 4000, Settings: `{"clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeL2TPSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	if ib.Port != 1701 {
		t.Errorf("port = %d; L2TP clients always dial UDP 1701", ib.Port)
	}
	var saved map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &saved); err != nil {
		t.Fatal(err)
	}
	psk, _ := saved["psk"].(string)
	if saved["pool"] != "10.90.1.0/24" || len(psk) < 16 {
		t.Errorf("pool = %v, psk = %q; want the first free /24 after IKEv2's and a generated key", saved["pool"], psk)
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}

	// An edit that sends the key back unchanged must not rotate it on every phone.
	saved["dns"] = []any{"9.9.9.9"}
	edited, _ := json.Marshal(saved)
	again := &model.Inbound{Id: 2, Protocol: model.L2TP, Settings: string(edited)}
	if err := svc.normalizeL2TPSettings(again, ib.Settings); err != nil {
		t.Fatal(err)
	}
	inst, _, ok := l2tp.ServerOf(again)
	if !ok || inst.PSK != psk || inst.DNS[0] != "9.9.9.9" {
		t.Fatalf("after an edit: %+v", inst)
	}
}

func TestDesiredL2TPInstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.L2TP, Settings: `{"clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeL2TPSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "l2tp-1", "", 1701, model.L2TP, "", ib.Settings)
	id := loadInboundByTag(t, "l2tp-1").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredL2TPInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}

// UDP 1701, the host's chap-secrets and the routed pool are host-wide.
func TestHostedPanelCannotRunL2TP(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	err := (&InboundService{}).normalizeL2TPSettings(&model.Inbound{Protocol: model.L2TP, Settings: `{}`}, "")
	if !errors.Is(err, ErrHostedForbidden) {
		t.Fatalf("err = %v, want ErrHostedForbidden", err)
	}
}

func TestOtherVPNsAvoidTheL2TPPool(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "l2tp-taken", "", 1701, model.L2TP, "", `{"pool":"10.89.0.0/24","psk":"k"}`)
	ib := &model.Inbound{Protocol: model.IKEv2, Settings: `{"serverAddress":"vpn.example.com"}`}
	if err := (&InboundService{}).normalizeIKEv2Settings(ib, ""); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	_ = json.Unmarshal([]byte(ib.Settings), &saved)
	if saved["pool"] != "10.89.1.0/24" {
		t.Fatalf("ikev2 pool = %v; it overlaps the L2TP inbound's 10.89.0.0/24", saved["pool"])
	}
}
