package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
)

func TestSSHSaveGeneratesHostKeyAndPasswordsAndKeepsThemAcrossEdits(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}

	ib := &model.Inbound{Protocol: model.SSH, Port: 2222, Settings: `{"clients":[{"email":"a","enable":true,"totalGB":5}]}`}
	if err := svc.normalizeSSHSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &saved); err != nil {
		t.Fatal(err)
	}
	key, _ := saved["hostKey"].(string)
	if sshtunnel.Fingerprint(key) == "" {
		t.Fatalf("hostKey = %q; want a parseable generated key", key)
	}
	client := saved["clients"].([]any)[0].(map[string]any)
	if pw, _ := client["password"].(string); len(pw) < 12 || client["totalGB"] != float64(5) {
		t.Fatalf("client = %v; want a generated password and its other fields kept", client)
	}

	// An edit that omits or garbles the key must not change the server's identity on every phone.
	for name, edited := range map[string]string{
		"omitted": `{"banner":"hi","clients":[]}`,
		"garbled": `{"hostKey":"not a key","clients":[]}`,
	} {
		again := &model.Inbound{Id: 2, Protocol: model.SSH, Port: 2222, Settings: edited}
		if err := svc.normalizeSSHSettings(again, ib.Settings); err != nil {
			t.Fatal(err)
		}
		inst, _, ok := sshtunnel.ServerOf(again)
		if !ok || inst.HostKey != key {
			t.Errorf("%s: host key changed on edit", name)
		}
	}
}

func TestSSHInboundNeedsAPort(t *testing.T) {
	setupConflictDB(t)
	err := (&InboundService{}).normalizeSSHSettings(&model.Inbound{Protocol: model.SSH, Settings: `{}`}, "")
	if err == nil || err.Error() != "ssh: a port is required" {
		t.Fatalf("err = %v, want the missing-port error", err)
	}
}

func TestDesiredSSHInstancesLeaveOutDepletedUsers(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ib := &model.Inbound{Protocol: model.SSH, Port: 2222, Settings: `{"clients":[` +
		`{"email":"alice","password":"alice-pass","enable":true},` +
		`{"email":"bob","password":"bob-pass","enable":true}]}`}
	if err := svc.normalizeSSHSettings(ib, ""); err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "ssh-2222", "", 2222, model.SSH, "", ib.Settings)
	id := loadInboundByTag(t, "ssh-2222").Id
	seedClientTraffic(t, id, "alice", true)
	seedClientTraffic(t, id, "bob", false)

	desired, err := svc.DesiredSSHInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || len(desired[0].Users) != 1 || desired[0].Users["alice"].Password != "alice-pass" {
		t.Fatalf("desired = %+v; want only alice, bob is over his quota", desired)
	}
}
