package service

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func remainingClients(t *testing.T) string {
	t.Helper()
	var emails []string
	if err := database.GetDB().Model(&model.ClientRecord{}).Pluck("email", &emails).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(emails)
	return strings.Join(emails, ",")
}

// Clients are deleted N days after they ran out, counted from the expiry
// itself or from when the quota was first seen used up.
func TestAutoDeleteDepletedAfterDays(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	now := time.Now()
	day := 24 * time.Hour
	for _, c := range []model.Client{
		{Email: "d-time", ExpiryTime: now.Add(-day).UnixMilli(), Enable: true},
		{Email: "d-quota", TotalGB: 1000, Enable: true},
		{Email: "d-live", TotalGB: 1000, Enable: true},
		{Email: "d-monthly", TotalGB: 1000, Enable: true},
	} {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: c, InboundIds: []int{id}}); err != nil {
			t.Fatal(err)
		}
	}
	setClientUsage(t, "d-quota", 1000)
	setClientUsage(t, "d-monthly", 1000)
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "d-monthly").Update("reset", 30).Error; err != nil {
		t.Fatal(err)
	}
	cleanup := &ClientCleanupService{}
	run := func(at time.Time) {
		t.Helper()
		if _, _, err := cleanup.Run(inbounds, at); err != nil {
			t.Fatal(err)
		}
	}

	run(now)
	if got := remainingClients(t); got != "d-live,d-monthly,d-quota,d-time" {
		t.Fatalf("auto-delete off by default, yet clients = %s", got)
	}
	if err := (&SettingService{}).setInt("autoDeleteDepletedDays", 3); err != nil {
		t.Fatal(err)
	}
	run(now.Add(2*day + time.Hour))
	if got := remainingClients(t); got != "d-live,d-monthly,d-quota" {
		t.Fatalf("expired 3 days ago should go, used up 2 days ago stay; clients = %s", got)
	}
	setClientUsage(t, "d-quota", 0) // renewed after its 3 days were up, before the next run
	run(now.Add(3*day + time.Hour))
	if got := remainingClients(t); got != "d-live,d-monthly,d-quota" {
		t.Fatalf("a client renewed since the last run must survive; clients = %s", got)
	}
	setClientUsage(t, "d-quota", 1000)
	run(now.Add(4 * day))
	if got := remainingClients(t); got != "d-live,d-monthly,d-quota" {
		t.Fatalf("a renewal restarts the count; clients = %s", got)
	}
	run(now.Add(7*day + time.Minute))
	if got := remainingClients(t); got != "d-live,d-monthly" {
		t.Fatalf("clients = %s; a periodically reset client is never deleted", got)
	}
}
