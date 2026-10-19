package service

import (
	"encoding/json"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func putClientIPs(t *testing.T, email string, nowSec int64, ips ...string) {
	t.Helper()
	entries := make([]clientIpEntry, 0, len(ips))
	for _, ip := range ips {
		entries = append(entries, clientIpEntry{IP: ip, Timestamp: nowSec})
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	row := &model.InboundClientIps{ClientEmail: email, Ips: string(raw)}
	if err := database.GetDB().Where("client_email = ?", email).Assign(row).FirstOrCreate(&model.InboundClientIps{}).Error; err != nil {
		t.Fatal(err)
	}
}

func sharedAccountService(t *testing.T, minIPs, minCountries int, countries map[string]string) *SharedAccountService {
	t.Helper()
	settings := &SettingService{}
	for k, v := range map[string]int{"sharedAccountMinIPs": minIPs, "sharedAccountMinCountries": minCountries, "sharedAccountWindowHours": 24} {
		if err := settings.setInt(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return &SharedAccountService{countryOf: func(a netip.Addr) string { return countries[a.String()] }}
}

func collectSharedEvents(t *testing.T) func() []*eventbus.SharedAccountData {
	t.Helper()
	bus := eventbus.New(0)
	var mu sync.Mutex
	var got []*eventbus.SharedAccountData
	bus.Subscribe("shared-test", func(e eventbus.Event) {
		if d, ok := e.Data.(*eventbus.SharedAccountData); ok && e.Type == eventbus.EventClientSharedAccount {
			mu.Lock()
			got = append(got, d)
			mu.Unlock()
		}
	})
	SetEventBus(bus)
	t.Cleanup(func() {
		SetEventBus(nil)
		bus.Stop()
	})
	return func() []*eventbus.SharedAccountData {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]*eventbus.SharedAccountData(nil), got...)
	}
}

func TestManyAddressesFlagTheClientAndAlertOnce(t *testing.T) {
	setupBulkDB(t)
	events := collectSharedEvents(t)
	s := sharedAccountService(t, 3, 5, map[string]string{"203.0.113.1": "DE", "203.0.113.2": "DE", "203.0.113.3": "US"})
	now := time.Now()
	putClientIPs(t, "alice", now.Unix(), "203.0.113.1", "203.0.113.2", "203.0.113.3")
	putClientIPs(t, "bob", now.Unix(), "203.0.113.1", "203.0.113.2")

	for range 2 {
		if err := s.Scan(now); err != nil {
			t.Fatal(err)
		}
	}
	flags, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(flags) != 1 || flags[0].Email != "alice" || flags[0].IPCount != 3 || flags[0].CountryCount != 2 || flags[0].Countries != "DE,US" {
		t.Fatalf("flags = %+v, want only alice with 3 addresses in DE,US", flags)
	}
	got := events()
	if len(got) != 1 || got[0].Email != "alice" || got[0].IPCount != 3 {
		t.Fatalf("events = %+v, want exactly one alert for alice", got)
	}
}

func TestManyCountriesFlagEvenWithFewAddresses(t *testing.T) {
	setupBulkDB(t)
	s := sharedAccountService(t, 0, 2, map[string]string{"203.0.113.1": "DE", "198.51.100.1": "BR"})
	now := time.Now()
	putClientIPs(t, "carol", now.Unix(), "203.0.113.1", "198.51.100.1")

	if err := s.Scan(now); err != nil {
		t.Fatal(err)
	}
	flags, _ := s.List()
	if len(flags) != 1 || flags[0].Email != "carol" || flags[0].CountryCount != 2 {
		t.Fatalf("flags = %+v, want carol flagged for two countries", flags)
	}
}

func TestSightingsAccumulateAcrossTheWindowBeyondTheLiveIPList(t *testing.T) {
	setupBulkDB(t)
	s := sharedAccountService(t, 3, 0, nil)
	now := time.Now()
	putClientIPs(t, "dave", now.Unix(), "203.0.113.1")
	_ = s.Scan(now)
	putClientIPs(t, "dave", now.Unix(), "203.0.113.2")
	_ = s.Scan(now.Add(2 * time.Hour))
	putClientIPs(t, "dave", now.Unix(), "203.0.113.3")
	_ = s.Scan(now.Add(4 * time.Hour))

	flags, _ := s.List()
	if len(flags) != 1 || flags[0].Email != "dave" || flags[0].IPCount != 3 {
		t.Fatalf("flags = %+v, want dave flagged after three addresses over four hours", flags)
	}
}

func TestTheFlagClearsOnceSightingsLeaveTheWindowAndAlertsAgainOnReturn(t *testing.T) {
	setupBulkDB(t)
	events := collectSharedEvents(t)
	s := sharedAccountService(t, 2, 0, nil)
	now := time.Now()
	putClientIPs(t, "erin", now.Unix(), "203.0.113.1", "203.0.113.2")
	_ = s.Scan(now)

	if err := database.GetDB().Where("1 = 1").Delete(&model.InboundClientIps{}).Error; err != nil {
		t.Fatal(err)
	}
	later := now.Add(25 * time.Hour)
	_ = s.Scan(later)
	if flags, _ := s.List(); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none once the window passed", flags)
	}

	putClientIPs(t, "erin", later.Unix(), "203.0.113.1", "203.0.113.2")
	_ = s.Scan(later)
	if got := events(); len(got) != 2 {
		t.Fatalf("%d alerts, want one for each time the client crossed the threshold", len(got))
	}
}

func TestNothingIsRecordedWhileBothThresholdsAreOff(t *testing.T) {
	setupBulkDB(t)
	s := sharedAccountService(t, 0, 0, nil)
	now := time.Now()
	putClientIPs(t, "frank", now.Unix(), "203.0.113.1", "203.0.113.2", "203.0.113.3")

	if err := s.Scan(now); err != nil {
		t.Fatal(err)
	}
	var n int64
	database.GetDB().Model(&model.ClientIPSighting{}).Count(&n)
	if flags, _ := s.List(); len(flags) != 0 || n != 0 {
		t.Fatalf("%d flags and %d sightings with the feature off", len(flags), n)
	}
}
