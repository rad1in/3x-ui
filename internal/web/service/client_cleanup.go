package service

import (
	"sort"
	"time"

	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// ClientCleanupService deletes clients that stayed out of quota or past their
// expiry for autoDeleteDepletedDays. Clients with a periodic reset never qualify.
type ClientCleanupService struct {
	settings SettingService
	clients  ClientService
}

func (s *ClientCleanupService) Run(inboundSvc *InboundService, now time.Time) (deleted int, needRestart bool, err error) {
	nowMs := now.UnixMilli()
	db := database.GetDB()
	var rows []xray.ClientTraffic
	if err := db.Where(depletedClientsClause, nowMs).Find(&rows).Error; err != nil {
		return 0, false, err
	}
	var tracked []model.ClientDepletion
	if err := db.Find(&tracked).Error; err != nil {
		return 0, false, err
	}
	since := make(map[string]int64, len(tracked))
	for _, d := range tracked {
		since[d.Email] = d.Since
	}

	depleted := make(map[string]bool, len(rows))
	var added []model.ClientDepletion
	for _, r := range rows {
		if r.Email == "" || depleted[r.Email] {
			continue
		}
		depleted[r.Email] = true
		if _, ok := since[r.Email]; ok {
			continue
		}
		start := nowMs
		if r.ExpiryTime > 0 && r.ExpiryTime <= nowMs {
			start = r.ExpiryTime
		}
		since[r.Email] = start
		added = append(added, model.ClientDepletion{Email: r.Email, Since: start})
	}
	var recovered []string
	for email := range since {
		if !depleted[email] {
			recovered = append(recovered, email)
			delete(since, email)
		}
	}
	if len(added) > 0 {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&added).Error; err != nil {
			return 0, false, err
		}
	}
	for _, batch := range chunkStrings(recovered, sqlInChunk) {
		if err := db.Where("email IN ?", batch).Delete(&model.ClientDepletion{}).Error; err != nil {
			return 0, false, err
		}
	}

	days, _ := s.settings.getInt("autoDeleteDepletedDays")
	if days <= 0 {
		return 0, false, nil
	}
	cutoff := nowMs - int64(days)*dayMs
	var due []string
	for email, start := range since {
		if start <= cutoff {
			due = append(due, email)
		}
	}
	if len(due) == 0 {
		return 0, false, nil
	}
	sort.Strings(due)
	res, needRestart, err := s.clients.BulkDelete(inboundSvc, due, false)
	return res.Deleted, needRestart, err
}
