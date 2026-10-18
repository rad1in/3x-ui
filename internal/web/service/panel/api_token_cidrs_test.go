package panel

import "testing"

func TestAPITokenAllowedCidrs(t *testing.T) {
	setupAPITokenTestDB(t)
	svc := &ApiTokenService{}

	if _, err := svc.CreateRestricted("bad", "admin", 0, "10.0.0.0/8, not-an-ip"); err == nil {
		t.Fatal("an unreadable entry was accepted; a typo would leave the token unrestricted")
	}
	created, err := svc.CreateRestricted("ci", "admin", 0, " 203.0.113.0/24 , 198.51.100.4 ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.AllowedCidrs != "203.0.113.0/24 , 198.51.100.4" {
		t.Fatalf("stored list = %q, want it trimmed", created.AllowedCidrs)
	}

	// The CLI rotates tokens by name; rotation must not drop the restriction.
	if _, err := svc.RecreateByName("ci", ""); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list after rotation: %v %v", list, err)
	}
	if list[0].AllowedCidrs != "203.0.113.0/24 , 198.51.100.4" {
		t.Fatalf("rotation changed the list to %q", list[0].AllowedCidrs)
	}

	if err := svc.SetAllowedCidrs(list[0].Id, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if list, _ = svc.List(); list[0].AllowedCidrs != "" {
		t.Fatalf("list after clearing = %q", list[0].AllowedCidrs)
	}
	if err := svc.SetAllowedCidrs(list[0].Id, "300.1.1.1"); err == nil {
		t.Fatal("an invalid address was accepted on update")
	}
}
