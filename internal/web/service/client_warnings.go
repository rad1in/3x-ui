package service

import (
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const dayMs = int64(24 * time.Hour / time.Millisecond)

// ClientWarningService sends client.quota_warning when a client's usage reaches
// quotaWarnPercent of its quota, and client.expiry_warning expireDiff days
// before it expires, once per crossing.
type ClientWarningService struct {
	settings SettingService
}

type warningCandidate struct {
	Email      string
	SubID      string `gorm:"column:sub_id"`
	Total      int64  `gorm:"column:total_gb"`
	ExpiryTime int64  `gorm:"column:expiry_time"`
	Up, Down   int64
}

func (c warningCandidate) used() int64 { return c.Up + c.Down }

func (c warningCandidate) quotaDue(percent int) bool {
	return percent > 0 && c.Total > 0 && c.used()*100 >= c.Total*int64(percent)
}

func (c warningCandidate) expiryDue(nowMs int64, days int) bool {
	return days > 0 && c.ExpiryTime > 0 && c.ExpiryTime-nowMs <= int64(days)*dayMs
}

func (c warningCandidate) event(nowMs int64) eventbus.ClientEventData {
	d := eventbus.ClientEventData{Email: c.Email, SubID: c.SubID, Up: c.Up, Down: c.Down, Total: c.Total, ExpiryTime: c.ExpiryTime}
	if c.Total > 0 {
		d.Percent = int(c.used() * 100 / c.Total)
	}
	if c.ExpiryTime > 0 {
		d.DaysLeft = int((c.ExpiryTime - nowMs + dayMs - 1) / dayMs)
	}
	return d
}

// Check runs one pass at the given time. The first pass ever only records
// who is already past a line, so turning the feature on sends no backlog.
func (s *ClientWarningService) Check(now time.Time) error {
	percent, _ := s.settings.GetQuotaWarnPercent()
	days, _ := s.settings.GetExpireDiff()
	seeded, _ := s.settings.getBool("clientWarningsSeeded")
	nowMs := now.UnixMilli()

	db := database.GetDB()
	q := newClientQuery(db, nowMs, 0, 0)
	due := map[string]warningCandidate{}
	var lines []string
	var args []any
	if percent > 0 {
		lines = append(lines, "(c.total_gb > 0 AND "+q.usedExpr+" * 100 >= c.total_gb * ?)")
		args = append(args, percent)
	}
	if days > 0 {
		lines = append(lines, "(c.expiry_time > 0 AND c.expiry_time - ? <= ?)")
		args = append(args, nowMs, int64(days)*dayMs)
	}
	if len(lines) > 0 {
		var rows []warningCandidate
		err := q.from().
			Select("c.email AS email, c.sub_id, c.total_gb, c.expiry_time, "+q.upExpr+" AS up, "+q.downExpr+" AS down").
			Where(sqlClientEnabled+" AND NOT "+q.depletedExpr()).
			Where(strings.Join(lines, " OR "), args...).
			Scan(&rows).Error
		if err != nil {
			return err
		}
		for _, r := range rows {
			due[r.Email] = r
		}
	}

	var sent []model.ClientWarning
	if err := db.Find(&sent).Error; err != nil {
		return err
	}
	state := make(map[string]model.ClientWarning, len(sent))
	for _, w := range sent {
		state[w.Email] = w
	}

	var upserts []model.ClientWarning
	var events []eventbus.Event
	for email, c := range due {
		prev := state[email]
		next := model.ClientWarning{Email: email, Quota: c.quotaDue(percent), Expiry: c.expiryDue(nowMs, days)}
		if next == prev {
			continue
		}
		upserts = append(upserts, next)
		data := c.event(nowMs)
		if next.Quota && !prev.Quota {
			events = append(events, clientWarningEvent(eventbus.EventClientQuotaWarning, data, now))
		}
		if next.Expiry && !prev.Expiry {
			events = append(events, clientWarningEvent(eventbus.EventClientExpiryWarning, data, now))
		}
	}
	var cleared []string
	for email := range state {
		if _, ok := due[email]; !ok {
			cleared = append(cleared, email)
		}
	}

	if len(upserts) > 0 {
		if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&upserts).Error; err != nil {
			return err
		}
	}
	for _, batch := range chunkStrings(cleared, sqlInChunk) {
		if err := db.Where("email IN ?", batch).Delete(&model.ClientWarning{}).Error; err != nil {
			return err
		}
	}
	if !seeded {
		if err := s.settings.setBool("clientWarningsSeeded", true); err != nil {
			logger.Warning("client warnings: mark seeded:", err)
		}
		return nil
	}
	if eventBus != nil {
		for _, e := range events {
			eventBus.Publish(e)
		}
	}
	return nil
}

func clientWarningEvent(t eventbus.EventType, d eventbus.ClientEventData, at time.Time) eventbus.Event {
	return eventbus.Event{Type: t, Source: d.Email, Data: &d, Timestamp: at}
}
