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

func seedWeeklyClient(t *testing.T, email string, c model.ClientRecord, up, down int64) {
	t.Helper()
	c.Email = email
	c.Enable = true
	db := database.GetDB()
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: email, Enable: true, Up: up, Down: down, Total: c.TotalGB}).Error; err != nil {
		t.Fatal(err)
	}
}

func setWeeklyUsage(t *testing.T, email string, used int64) {
	t.Helper()
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"up": used, "down": 0}).Error; err != nil {
		t.Fatal(err)
	}
}

// Traffic is the growth since the last report, a reset counter counts whole, and clients are classified by this week.
func TestWeeklySummaryCountsGrowthResetsAndClientChanges(t *testing.T) {
	setupConflictDB(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()
	day := int64(24 * time.Hour / time.Millisecond)
	seedWeeklyClient(t, "heavy", model.ClientRecord{CreatedAt: nowMs - 30*day}, 5000, 0)
	seedWeeklyClient(t, "reset", model.ClientRecord{CreatedAt: nowMs - 30*day}, 100, 0)
	seedWeeklyClient(t, "fresh", model.ClientRecord{CreatedAt: nowMs - 2*day}, 40, 0)
	seedWeeklyClient(t, "lapsed", model.ClientRecord{CreatedAt: nowMs - 30*day, ExpiryTime: nowMs - day}, 0, 0)
	seedWeeklyClient(t, "full", model.ClientRecord{CreatedAt: nowMs - 30*day, TotalGB: 1000}, 1000, 0)

	baseline := map[string]int64{"heavy": 3000, "reset": 900, "lapsed": 0, "full": 1000}
	d, next, err := buildWeeklySummary(now, nowMs-7*day, baseline)
	if err != nil {
		t.Fatal(err)
	}
	// heavy +2000, reset 100 (counter went down), fresh 40 (no baseline), full +0.
	if d.Bytes != 2140 || d.FirstReport {
		t.Fatalf("bytes=%d first=%v", d.Bytes, d.FirstReport)
	}
	if d.NewClients != 1 || d.ExpiredClients != 1 || d.DepletedClients != 1 || d.TotalClients != 5 {
		t.Fatalf("clients: %+v", d)
	}
	if len(d.TopUsers) != 3 || d.TopUsers[0].Email != "heavy" || d.TopUsers[1].Email != "reset" || d.TopUsers[2].Email != "fresh" {
		t.Fatalf("top users: %+v", d.TopUsers)
	}
	if next["heavy"] != 5000 || next["reset"] != 100 {
		t.Fatalf("next baseline: %v", next)
	}

	first, _, err := buildWeeklySummary(now, 0, nil)
	if err != nil || !first.FirstReport {
		t.Fatalf("err=%v first=%+v", err, first)
	}
}

// Due fires only in the configured hour, and a restart inside that hour must not send it again.
func TestWeeklySummaryDueAndRunPublishesOnce(t *testing.T) {
	setupConflictDB(t)
	log := captureClientEvents(t)
	s := &WeeklySummaryService{}
	for k, v := range map[string]string{"weeklyReportEnable": "true", "weeklyReportDay": "1", "weeklyReportHour": "9"} {
		if err := s.settings.saveSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	monday9 := time.Date(2026, 10, 19, 9, 5, 0, 0, time.UTC)
	if s.Due(monday9.Add(-24*time.Hour)) || s.Due(monday9.Add(time.Hour)) {
		t.Fatal("due outside the configured day/hour")
	}
	if !s.Due(monday9) {
		t.Fatal("not due at the configured hour")
	}
	seedWeeklyClient(t, "a", model.ClientRecord{}, 10, 0)
	if err := s.Run(monday9); err != nil {
		t.Fatal(err)
	}
	if got := log.settled(t, 1); got != "report.weekly:weekly" {
		t.Fatalf("events = %s", got)
	}
	if s.Due(monday9.Add(10 * time.Minute)) {
		t.Fatal("due again right after sending")
	}
	if !s.Due(monday9.Add(7 * 24 * time.Hour)) {
		t.Fatal("not due the next week")
	}
	if text := strings.Join((&eventbus.WeeklySummaryData{Bytes: 1 << 20}).Lines(), "\n"); !strings.Contains(text, "1.00MB") {
		t.Fatalf("text = %q", text)
	}
}
