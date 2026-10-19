package service

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func alertService(t *testing.T, countries map[string]string) *LoginAlertService {
	t.Helper()
	if err := (&SecurityConfigService{}).Save(&SecurityConfig{NewLocationAlert: true}); err != nil {
		t.Fatal(err)
	}
	return &LoginAlertService{countryOf: func(a netip.Addr) string { return countries[a.String()] }}
}

func collectLocationEvents(t *testing.T) func() []*eventbus.NewLoginLocationData {
	t.Helper()
	bus := eventbus.New(0)
	var mu sync.Mutex
	var got []*eventbus.NewLoginLocationData
	bus.Subscribe("loc-test", func(e eventbus.Event) {
		if d, ok := e.Data.(*eventbus.NewLoginLocationData); ok && e.Type == eventbus.EventLoginNewLocation {
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
	return func() []*eventbus.NewLoginLocationData {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]*eventbus.NewLoginLocationData(nil), got...)
	}
}

func adminUser(t *testing.T) *model.User {
	t.Helper()
	u := &model.User{Username: "root", Password: "x", Role: model.AdminRoleOwner, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	if err := database.GetDB().Create(u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func TestTheFirstLoginOnlyLearnsTheLocation(t *testing.T) {
	setupBulkDB(t)
	events := collectLocationEvents(t)
	s := alertService(t, map[string]string{"203.0.113.5": "DE"})
	u := adminUser(t)

	s.Observe(u, "203.0.113.5")
	if got := events(); len(got) != 0 {
		t.Fatalf("the very first login raised %d alerts, it should only be learned", len(got))
	}
}

func TestANewAddressAlertsOnceAndIsThenKnown(t *testing.T) {
	setupBulkDB(t)
	events := collectLocationEvents(t)
	s := alertService(t, map[string]string{"203.0.113.5": "DE", "203.0.113.6": "DE"})
	u := adminUser(t)

	s.Observe(u, "203.0.113.5")
	s.Observe(u, "203.0.113.6")
	s.Observe(u, "203.0.113.6")
	s.Observe(u, "203.0.113.5")
	got := events()
	if len(got) != 1 {
		t.Fatalf("%d alerts, want one for the new address only", len(got))
	}
	if d := got[0]; d.Username != "root" || d.IP != "203.0.113.6" || d.Country != "DE" || d.NewCountry {
		t.Fatalf("alert = %+v, want a new address in the known country DE", d)
	}
}

func TestANewCountryIsFlaggedAsSuch(t *testing.T) {
	setupBulkDB(t)
	events := collectLocationEvents(t)
	s := alertService(t, map[string]string{"203.0.113.5": "DE", "198.51.100.9": "BR"})
	u := adminUser(t)

	s.Observe(u, "203.0.113.5")
	s.Observe(u, "198.51.100.9")
	got := events()
	if len(got) != 1 || !got[0].NewCountry || got[0].Country != "BR" {
		t.Fatalf("alerts = %+v, want one flagged as a new country BR", got)
	}
}

func TestNothingHappensWhenTheAlertIsOff(t *testing.T) {
	setupBulkDB(t)
	events := collectLocationEvents(t)
	s := &LoginAlertService{countryOf: func(netip.Addr) string { return "DE" }}
	u := adminUser(t)

	s.Observe(u, "203.0.113.5")
	s.Observe(u, "203.0.113.6")
	if got := events(); len(got) != 0 {
		t.Fatalf("%d alerts with the feature off", len(got))
	}
	var n int64
	database.GetDB().Model(&model.LoginSeen{}).Count(&n)
	if n != 0 {
		t.Fatalf("%d locations stored with the feature off", n)
	}
}

func TestUnparsableAddressesAreIgnored(t *testing.T) {
	setupBulkDB(t)
	events := collectLocationEvents(t)
	s := alertService(t, nil)
	u := adminUser(t)

	s.Observe(u, "203.0.113.5")
	s.Observe(u, "unknown")
	s.Observe(u, "")
	if got := events(); len(got) != 0 {
		t.Fatalf("%d alerts for an address that is not one", len(got))
	}
}
