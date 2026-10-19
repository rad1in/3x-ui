package service

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func geoService(countries map[string]string, calls *int) *ClientGeoService {
	return &ClientGeoService{countryOf: func(a netip.Addr) (string, error) {
		if calls != nil {
			*calls++
		}
		return countries[a.String()], nil
	}}
}

func TestClientGeoCountsClientsAndAddressesPerCountry(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	putClientIPs(t, "alice", now.Unix(), "203.0.113.1", "203.0.113.2")
	putClientIPs(t, "bob", now.Unix(), "203.0.113.1", "198.51.100.7")
	putClientIPs(t, "carol", now.Unix(), "192.0.2.9")
	putClientIPs(t, "stale", now.Add(-10*time.Minute).Unix(), "198.51.100.8")

	geo, err := geoService(map[string]string{"203.0.113.1": "DE", "203.0.113.2": "DE", "198.51.100.7": "BR", "198.51.100.8": "BR"}, nil).Snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	want := []CountryCount{{Country: "DE", Clients: 2, Addresses: 2}, {Country: "BR", Clients: 1, Addresses: 1}}
	if !geo.Available || len(geo.Countries) != 2 || geo.Countries[0] != want[0] || geo.Countries[1] != want[1] {
		t.Fatalf("geo = %+v, want %+v (the stale address excluded)", geo, want)
	}
	if geo.Unlocated != 1 {
		t.Fatalf("unlocated = %d, want carol's one address outside every country", geo.Unlocated)
	}
}

func TestClientGeoReportsAMissingDatabaseInsteadOfAnEmptyMap(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	putClientIPs(t, "alice", now.Unix(), "203.0.113.1")
	s := &ClientGeoService{countryOf: func(netip.Addr) (string, error) { return "", errors.New("no geoip.dat") }}

	geo, err := s.Snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if geo.Available || len(geo.Countries) != 0 {
		t.Fatalf("geo = %+v, want unavailable with no countries", geo)
	}
}

func TestClientGeoLooksEachAddressUpOnceAcrossSnapshots(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	putClientIPs(t, "alice", now.Unix(), "203.0.113.1")
	putClientIPs(t, "bob", now.Unix(), "203.0.113.1")
	calls := 0
	s := geoService(map[string]string{"203.0.113.1": "DE"}, &calls)

	for range 3 {
		if _, err := s.Snapshot(now); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("%d lookups, want 1: each lookup scans the whole database", calls)
	}
}
