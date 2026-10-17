package hosted

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLimitsRoundTripThroughTheHostedDir(t *testing.T) {
	dir := t.TempDir()
	want := Limits{MaxInbounds: 3, MaxClients: 50, TrafficBytes: 100 << 30, ExpiresAt: 1790000000000, PortMin: 20000, PortMax: 20999, UsageEpoch: 2}
	if err := WriteLimits(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLimits(dir)
	if err != nil || got != want {
		t.Fatalf("ReadLimits = %+v, %v; want %+v", got, err, want)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("atomic write left temp files behind: %v", entries)
	}
}

func TestMissingLimitsFileDeniesEverything(t *testing.T) {
	_, err := ReadLimits(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("a hosted panel without a limits file must not run unlimited")
	}
}

func TestLimitsPortAndExpiry(t *testing.T) {
	l := Limits{PortMin: 20000, PortMax: 20010, ExpiresAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).UnixMilli()}
	for port, want := range map[int]bool{19999: false, 20000: true, 20010: true, 20011: false} {
		if got := l.PortAllowed(port); got != want {
			t.Errorf("PortAllowed(%d) = %v, want %v", port, got, want)
		}
	}
	if !(Limits{}).PortAllowed(443) {
		t.Error("no range configured must allow any port")
	}
	if l.Expired(time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC)) || !l.Expired(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Error("expiry boundary is wrong")
	}
	if (Limits{}).Expired(time.Now()) {
		t.Error("zero ExpiresAt means never")
	}
}

func TestStatusRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := PanelStatus{Inbounds: 2, Clients: 9, UsedBytes: 12345, Blocked: BlockedTraffic, XrayRunning: true, UpdatedAt: 99}
	if err := WriteStatus(dir, want); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadStatus(dir); err != nil || got != want {
		t.Fatalf("ReadStatus = %+v, %v", got, err)
	}
}
