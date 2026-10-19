package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	importGiB   = int64(1) << 30
	dayMillis   = int64(24 * 3600 * 1000)
	utc20270101 = int64(1798761600000)
)

func sampleExport(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "import", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func byEmail(t *testing.T, items []ClientCreatePayload) map[string]model.Client {
	t.Helper()
	out := map[string]model.Client{}
	for _, it := range items {
		if _, dup := out[it.Client.Email]; dup {
			t.Fatalf("duplicate email %q in conversion", it.Client.Email)
		}
		out[it.Client.Email] = it.Client
	}
	return out
}

func TestConvertMarzbanKeepsCredentialsQuotaAndExpiry(t *testing.T) {
	items, warnings, err := ConvertForeignUsers(ForeignMarzban, sampleExport(t, "marzban_users.json"), []int{4})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("converted %d users, want 5", len(items))
	}
	for _, it := range items {
		if len(it.InboundIds) != 1 || it.InboundIds[0] != 4 {
			t.Fatalf("%s attached to %v, want [4]", it.Client.Email, it.InboundIds)
		}
	}
	c := byEmail(t, items)

	alice := c["alice"]
	if alice.ID != "8f2a6c1e-3b7d-4e55-9a10-2c4d5e6f7a81" || alice.Flow != "xtls-rprx-vision" || alice.Password != "Tr0jan-p4ss-alice" {
		t.Fatalf("alice credentials = id %q flow %q pw %q", alice.ID, alice.Flow, alice.Password)
	}
	if alice.TotalGB != 100*importGiB || alice.ExpiryTime != utc20270101 || !alice.Enable {
		t.Fatalf("alice limits = %d / %d / enable %v", alice.TotalGB, alice.ExpiryTime, alice.Enable)
	}
	if alice.TrafficReset != "monthly" || alice.TrafficResetDay != 1 || alice.Comment != "paid until end of 2026" {
		t.Fatalf("alice reset = %q day %d comment %q", alice.TrafficReset, alice.TrafficResetDay, alice.Comment)
	}
	if tr := items[0].Traffic; tr == nil || tr.Up+tr.Down != 20*importGiB {
		t.Fatalf("alice traffic = %+v, want 20 GiB carried over", tr)
	}

	bob := c["bob_unlimited"]
	if bob.TotalGB != 0 || bob.ExpiryTime != 0 || bob.TrafficReset != "never" || bob.ID != "b17d9d0a-6a52-4b3c-8d2f-90a1c3e5f7b2" {
		t.Fatalf("bob = %+v", bob)
	}
	if carol := c["carol"]; carol.Enable || carol.Password != "carol-trojan-secret" {
		t.Fatalf("carol expired user must stay disabled with its trojan password: %+v", carol)
	}
	if dave := c["dave_hold"]; dave.ID != "d9a8b7c6-5e4f-4a3b-9c2d-0f1e2d3c4b5a" || dave.ExpiryTime != -30*dayMillis || !dave.Enable {
		t.Fatalf("on-hold user must become a delayed start of 30 days: %+v", dave)
	}
	erin := c["erin"]
	if erin.Enable || erin.TrafficReset != "never" {
		t.Fatalf("erin = %+v", erin)
	}
	var yearWarning bool
	for _, w := range warnings {
		if w.Email == "erin" {
			yearWarning = true
		}
	}
	if !yearWarning {
		t.Fatalf("no warning for erin's unsupported yearly reset: %+v", warnings)
	}
}

func TestConvertMarzneshinDerivesCredentialsFromKey(t *testing.T) {
	items, _, err := ConvertForeignUsers(ForeignMarzneshin, sampleExport(t, "marzneshin_users.json"), []int{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	c := byEmail(t, items)
	if len(c) != 3 {
		t.Fatalf("converted %d users, want 3", len(c))
	}
	frank := c["frank"]
	if frank.ID != "0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9" || frank.Password != "0a1b2c3d4e5f60718293a4b5c6d7e8f9" {
		t.Fatalf("frank credentials = %q / %q", frank.ID, frank.Password)
	}
	if frank.TotalGB != 50*importGiB || frank.TrafficReset != "weekly" || frank.Comment != "family plan" {
		t.Fatalf("frank = %+v", frank)
	}
	if want := int64(1798761599000); frank.ExpiryTime != want {
		t.Fatalf("frank expiry = %d, want %d (2026-12-31T23:59:59Z)", frank.ExpiryTime, want)
	}
	if grace := c["grace"]; grace.ExpiryTime != -15*dayMillis || grace.TotalGB != 0 {
		t.Fatalf("start-on-first-use user = %+v, want a 15 day delayed start", grace)
	}
	if heidi := c["heidi"]; heidi.Enable || heidi.ExpiryTime != 0 || heidi.TotalGB != importGiB {
		t.Fatalf("heidi = %+v", heidi)
	}
}

func TestConvertHiddifyUsesUUIDAsEveryCredential(t *testing.T) {
	items, _, err := ConvertForeignUsers(ForeignHiddify, sampleExport(t, "hiddify_users.json"), []int{1})
	if err != nil {
		t.Fatal(err)
	}
	c := byEmail(t, items)
	ivan := c["ivan"]
	const id = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	if ivan.ID != id || ivan.Password != id || ivan.Auth != id {
		t.Fatalf("ivan credentials = %q / %q / %q, want the uuid for all", ivan.ID, ivan.Password, ivan.Auth)
	}
	if ivan.TotalGB != 50*importGiB || ivan.LimitIP != 3 || ivan.TgID != 123456789 || ivan.TrafficReset != "monthly" {
		t.Fatalf("ivan = %+v", ivan)
	}
	// 2026-10-01 00:00 UTC + 30 days.
	if want := int64(1790812800000 + 30*dayMillis); ivan.ExpiryTime != want {
		t.Fatalf("ivan expiry = %d, want %d", ivan.ExpiryTime, want)
	}
	for _, it := range items {
		if it.Client.Email == "ivan" {
			if tr := it.Traffic; tr == nil || tr.Up+tr.Down != int64(12.5*float64(importGiB)) {
				t.Fatalf("ivan traffic = %+v, want 12.5 GiB", tr)
			}
		}
	}
	judy := c["judy"]
	if judy.ExpiryTime != -90*dayMillis || judy.LimitIP != 0 {
		t.Fatalf("judy = %+v, want a 90 day delayed start and the placeholder 1000 IP cap dropped", judy)
	}
	if mallory := c["mallory"]; mallory.Enable {
		t.Fatalf("mallory must stay disabled: %+v", mallory)
	}
}

func TestConvertForeignRejectsWrongFormatAndMissingInbounds(t *testing.T) {
	if _, _, err := ConvertForeignUsers(ForeignMarzban, []byte(`{"unexpected": true}`), []int{1}); err == nil {
		t.Fatal("a file without users was accepted as a Marzban export")
	}
	if _, _, err := ConvertForeignUsers(ForeignHiddify, sampleExport(t, "hiddify_users.json"), nil); err == nil {
		t.Fatal("import without target inbounds was accepted")
	}
	if _, _, err := ConvertForeignUsers("nope", []byte(`[]`), []int{1}); err == nil {
		t.Fatal("unknown source was accepted")
	}
}

func TestImportForeignUsersKeepsUUIDInInboundSettings(t *testing.T) {
	setupBulkDB(t)
	ib := mkInbound(t, 24001, model.VLESS, `{"clients":[]}`)

	res, needRestart, err := (&ClientService{}).ImportForeignClients(&InboundService{}, ForeignMarzban, sampleExport(t, "marzban_users.json"), []int{ib.Id})
	if err != nil {
		t.Fatalf("ImportForeignClients: %v", err)
	}
	_ = needRestart
	if res.Created != 5 {
		t.Fatalf("created = %d (%+v), want 5", res.Created, res.Skipped)
	}
	rec := lookupClientRecord(t, "alice")
	if rec.UUID != "8f2a6c1e-3b7d-4e55-9a10-2c4d5e6f7a81" || rec.TotalGB != 100*importGiB {
		t.Fatalf("stored alice = uuid %q total %d", rec.UUID, rec.TotalGB)
	}
	if !settingsHoldUUID(t, &InboundService{}, ib.Id, "8f2a6c1e-3b7d-4e55-9a10-2c4d5e6f7a81") {
		t.Fatal("inbound settings lost alice's original UUID")
	}
	if tr := trafficOf(t, "alice"); tr.Up+tr.Down != 20*importGiB {
		t.Fatalf("alice usage = %d, want 20 GiB", tr.Up+tr.Down)
	}

	again, _, err := (&ClientService{}).ImportForeignClients(&InboundService{}, ForeignMarzban, sampleExport(t, "marzban_users.json"), []int{ib.Id})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 || len(again.Skipped) != 5 {
		t.Fatalf("second import created %d skipped %d, want 0/5", again.Created, len(again.Skipped))
	}
}
