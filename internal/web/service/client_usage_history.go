package service

import (
	"maps"
	"slices"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ClientUsageRetention is how long per-day client usage rows are kept.
const ClientUsageRetention = 90 * 24 * time.Hour

const usageDayLayout = "2006-01-02"

type usageDelta struct{ up, down int64 }

// recordClientDailyUsage folds one tick's per-client bytes into today's rows
// with one upsert per chunk, so the traffic path pays a single extra statement.
func recordClientDailyUsage(tx *gorm.DB, deltas map[string]usageDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	day := time.Now().Format(usageDayLayout)
	rows := make([]model.ClientDailyUsage, 0, len(deltas))
	// Sorted so concurrent writers lock rows in one order on PostgreSQL.
	for _, email := range slices.Sorted(maps.Keys(deltas)) {
		d := deltas[email]
		if d.up == 0 && d.down == 0 {
			continue
		}
		rows = append(rows, model.ClientDailyUsage{Email: email, Day: day, Up: d.up, Down: d.down})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "email"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"up":   gorm.Expr("client_daily_usage.up + excluded.up"),
			"down": gorm.Expr("client_daily_usage.down + excluded.down"),
		}),
	}).CreateInBatches(rows, 200).Error
}

func renameClientUsage(tx *gorm.DB, oldEmail, newEmail string) error {
	if oldEmail == newEmail {
		return nil
	}
	return tx.Model(&model.ClientDailyUsage{}).Where("email = ?", oldEmail).Update("email", newEmail).Error
}

func deleteClientUsage(tx *gorm.DB, emails []string) error {
	for start := 0; start < len(emails); start += sqlInChunk {
		end := min(start+sqlInChunk, len(emails))
		if err := tx.Where("email IN ?", emails[start:end]).Delete(&model.ClientDailyUsage{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetClientUsageHistory returns the client's per-day rows for the last `days`
// days (today included), oldest first. Days without traffic have no row.
func (s *ClientService) GetClientUsageHistory(email string, days int) ([]model.ClientDailyUsage, error) {
	if days < 1 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -(days - 1)).Format(usageDayLayout)
	var rows []model.ClientDailyUsage
	err := database.GetDB().
		Where("email = ? AND day >= ?", email, since).
		Order("day asc").
		Find(&rows).Error
	return rows, err
}

// PruneClientUsageHistory drops days older than ClientUsageRetention.
func (s *ClientService) PruneClientUsageHistory(now time.Time) (int64, error) {
	cutoff := now.Add(-ClientUsageRetention).Format(usageDayLayout)
	res := database.GetDB().Where("day < ?", cutoff).Delete(&model.ClientDailyUsage{})
	return res.RowsAffected, res.Error
}
