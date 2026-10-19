package service

import (
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	// A subscription's log keeps its newest distinct fetchers, so a scanner
	// rotating IPs cannot grow the table without bound.
	subFetchKeep         = 20
	subFetchMaxUserAgent = 200
)

// SubFetchInfo is one app/IP that fetched a subscription.
type SubFetchInfo struct {
	IP        string `json:"ip" example:"198.51.100.7"`
	UserAgent string `json:"userAgent" example:"v2rayNG/1.10.2"`
	Format    string `json:"format" example:"raw"`
	Count     int    `json:"count" example:"12"`
	FirstSeen int64  `json:"firstSeen" example:"1735680000000"`
	LastSeen  int64  `json:"lastSeen" example:"1735766400000"`
}

type SubFetchService struct{}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// Record counts one successful subscription fetch under (ip, user agent, format).
func (s *SubFetchService) Record(subID, ip, userAgent, format string, at time.Time) error {
	subID = strings.TrimSpace(subID)
	if subID == "" {
		return nil
	}
	ms := at.UnixMilli()
	row := model.SubFetch{
		SubID: subID, IP: ip, UserAgent: truncateUTF8(userAgent, subFetchMaxUserAgent), Format: format,
		Count: 1, FirstSeen: ms, LastSeen: ms,
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sub_id"}, {Name: "ip"}, {Name: "user_agent"}, {Name: "format"}},
			DoUpdates: clause.Assignments(map[string]any{
				"count":     gorm.Expr("count + 1"),
				"last_seen": ms,
			}),
		}).Create(&row).Error
		if err != nil {
			return err
		}
		var stale []int
		if err := tx.Model(&model.SubFetch{}).Where("sub_id = ?", subID).
			Order("last_seen DESC, id DESC").Offset(subFetchKeep).Pluck("id", &stale).Error; err != nil || len(stale) == 0 {
			return err
		}
		return tx.Where("id IN ?", stale).Delete(&model.SubFetch{}).Error
	})
}

// List returns the subscription's fetchers, newest first.
func (s *SubFetchService) List(subID string) ([]SubFetchInfo, error) {
	var rows []model.SubFetch
	if err := database.GetDB().Where("sub_id = ?", subID).Order("last_seen DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SubFetchInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, SubFetchInfo{IP: r.IP, UserAgent: r.UserAgent, Format: r.Format, Count: r.Count, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen})
	}
	return out, nil
}
