package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

const (
	weeklyTopUsers = 5
	// A report counts as already sent for this long, so a restart inside the
	// scheduled hour cannot send it twice.
	weeklyMinGap = 6 * 24 * time.Hour
)

// WeeklySummaryService builds the weekly report from client usage and publishes
// report.weekly; each channel decides from its own event selection whether to send it.
type WeeklySummaryService struct {
	settings SettingService
}

type weeklyRow struct {
	Email      string
	Up, Down   int64
	CreatedAt  int64 `gorm:"column:created_at"`
	ExpiryTime int64 `gorm:"column:expiry_time"`
	TotalGB    int64 `gorm:"column:total_gb"`
}

// Due reports whether the configured weekday and hour (panel time) is now and no report went out lately.
func (s *WeeklySummaryService) Due(now time.Time) bool {
	if on, _ := s.settings.getBool("weeklyReportEnable"); !on {
		return false
	}
	day, _ := s.settings.getInt("weeklyReportDay")
	hour, _ := s.settings.getInt("weeklyReportHour")
	if int(now.Weekday()) != day || now.Hour() != hour {
		return false
	}
	return now.Sub(time.UnixMilli(s.lastSentMs())) >= weeklyMinGap
}

func (s *WeeklySummaryService) lastSentMs() int64 {
	raw, _ := s.settings.getString("weeklyReportLastSent")
	ms, _ := strconv.ParseInt(raw, 10, 64)
	return ms
}

// Run builds the summary, publishes it and rolls the traffic baseline forward.
func (s *WeeklySummaryService) Run(now time.Time) error {
	raw, _ := s.settings.getString("weeklyReportBaseline")
	var baseline map[string]int64
	_ = json.Unmarshal([]byte(raw), &baseline)
	lastMs := s.lastSentMs()

	data, next, err := buildWeeklySummary(now, lastMs, baseline)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := s.settings.setString("weeklyReportBaseline", string(blob)); err != nil {
		return err
	}
	if err := s.settings.setString("weeklyReportLastSent", strconv.FormatInt(now.UnixMilli(), 10)); err != nil {
		return err
	}
	if eventBus != nil {
		eventBus.Publish(eventbus.Event{Type: eventbus.EventWeeklyReport, Source: "weekly", Data: &data, Timestamp: now})
	}
	return nil
}

// buildWeeklySummary diffs current usage against baseline; a counter that went
// down was reset, so its whole value counts as the week's use.
func buildWeeklySummary(now time.Time, lastMs int64, baseline map[string]int64) (eventbus.WeeklySummaryData, map[string]int64, error) {
	since := now.Add(-7 * 24 * time.Hour).UnixMilli()
	if lastMs > 0 {
		since = lastMs
	}
	nowMs := now.UnixMilli()
	q := newClientQuery(database.GetDB(), nowMs, 0, 0)
	var rows []weeklyRow
	err := q.from().
		Select("c.email AS email, c.created_at, c.expiry_time, c.total_gb, " + q.upExpr + " AS up, " + q.downExpr + " AS down").
		Scan(&rows).Error
	if err != nil {
		return eventbus.WeeklySummaryData{}, nil, err
	}

	d := eventbus.WeeklySummaryData{SinceMs: since, UntilMs: nowMs, FirstReport: baseline == nil, TotalClients: len(rows)}
	next := make(map[string]int64, len(rows))
	var all []eventbus.TopUser
	for _, r := range rows {
		used := r.Up + r.Down
		next[r.Email] = used
		delta := used
		if prev, ok := baseline[r.Email]; ok && used >= prev {
			delta = used - prev
		}
		if delta > 0 {
			all = append(all, eventbus.TopUser{Email: r.Email, Bytes: delta})
			d.Bytes += delta
		}
		if r.CreatedAt >= since {
			d.NewClients++
		}
		if r.ExpiryTime > 0 && r.ExpiryTime >= since && r.ExpiryTime <= nowMs {
			d.ExpiredClients++
		}
		if r.TotalGB > 0 && used >= r.TotalGB {
			d.DepletedClients++
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Bytes > all[j].Bytes })
	d.TopUsers = all[:min(len(all), weeklyTopUsers)]
	return d, next, nil
}
