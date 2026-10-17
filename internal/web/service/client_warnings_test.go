package service

import (
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// settled returns every event once n arrived and no more came in shortly after.
func (l *eventLog) settled(t *testing.T, n int) string {
	t.Helper()
	l.wait(t, n)
	time.Sleep(150 * time.Millisecond)
	got := l.wait(t, n)
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
	return strings.Join(got, ",")
}

func setClientUsage(t *testing.T, email string, used int64) {
	t.Helper()
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"up": used, "down": 0}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestClientWarningsFireOncePerCrossing(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	settings := &SettingService{}
	if err := settings.setInt("quotaWarnPercent", 80); err != nil {
		t.Fatal(err)
	}
	if err := settings.setInt("expireDiff", 3); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, c := range []model.Client{
		{Email: "w-quota", TotalGB: 1000, Enable: true},
		{Email: "w-time", ExpiryTime: now.Add(10 * 24 * time.Hour).UnixMilli(), Enable: true},
		{Email: "w-old", TotalGB: 1000, Enable: true},
		{Email: "w-off", TotalGB: 1000, Enable: false},
		{Email: "w-both", TotalGB: 1000, ExpiryTime: now.Add(10 * 24 * time.Hour).UnixMilli(), Enable: true},
		{Email: "w-full", TotalGB: 1000, Enable: true},
	} {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: c, InboundIds: []int{id}}); err != nil {
			t.Fatal(err)
		}
	}
	setClientUsage(t, "w-old", 900)
	setClientUsage(t, "w-off", 900)
	log := captureClientEvents(t)
	warnings := &ClientWarningService{}
	check := func(at time.Time) {
		t.Helper()
		if err := warnings.Check(at); err != nil {
			t.Fatal(err)
		}
	}

	check(now)
	if got := log.settled(t, 0); got != "" {
		t.Fatalf("first run published %s; clients already past the line before the feature must be recorded silently", got)
	}
	setClientUsage(t, "w-quota", 850)
	setClientUsage(t, "w-both", 850)
	setClientUsage(t, "w-full", 1000) // depleted: the cut-off event covers it
	check(now)
	if got := log.settled(t, 2); got != "client.quota_warning:w-both,client.quota_warning:w-quota" {
		t.Fatalf("crossing 80%%: events = %q", got)
	}
	check(now)
	if got := log.settled(t, 0); got != "" {
		t.Fatalf("second run re-sent %s", got)
	}
	check(now.Add(8 * 24 * time.Hour))
	if got := log.settled(t, 2); got != "client.expiry_warning:w-both,client.expiry_warning:w-time" {
		t.Fatalf("3 days before expiry: events = %q", got)
	}

	setClientUsage(t, "w-quota", 0)
	check(now)
	if got := log.settled(t, 0); got != "" {
		t.Fatalf("after reset: events = %q", got)
	}
	setClientUsage(t, "w-quota", 900)
	check(now)
	log.wait(t, 1)
	d := log.lastData(t)
	if got := log.settled(t, 1); got != "client.quota_warning:w-quota" || d == nil || d.Percent != 90 {
		t.Fatalf("crossing again after a reset: events = %q, data = %+v", got, d)
	}
}

func TestClientWarningsReportDaysLeft(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	settings := &SettingService{}
	if err := settings.setInt("expireDiff", 3); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := clients.Create(inbounds, &ClientCreatePayload{
		Client: model.Client{Email: "w-days", ExpiryTime: now.Add(5 * 24 * time.Hour).UnixMilli(), Enable: true}, InboundIds: []int{id},
	}); err != nil {
		t.Fatal(err)
	}
	log := captureClientEvents(t)
	warnings := &ClientWarningService{}
	if err := warnings.Check(now); err != nil {
		t.Fatal(err)
	}
	if err := warnings.Check(now.Add(3*24*time.Hour + time.Hour)); err != nil {
		t.Fatal(err)
	}
	log.wait(t, 1)
	d := log.lastData(t)
	if d == nil || d.DaysLeft != 2 || d.Email != "w-days" {
		t.Fatalf("expiry warning data = %+v, want 2 days left (47h rounds up)", d)
	}
	if d.Percent != 0 {
		t.Fatalf("an unlimited client has no usage percent, got %d", d.Percent)
	}
}

func (l *eventLog) lastData(t *testing.T) *eventbus.ClientEventData {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.events) == 0 {
		return nil
	}
	d, _ := l.events[len(l.events)-1].Data.(*eventbus.ClientEventData)
	return d
}
