package job

import (
	"context"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ScheduledBackupJob writes the database backup to the configured folder and bucket.
type ScheduledBackupJob struct {
	backups service.ScheduledBackupService
}

func NewScheduledBackupJob() *ScheduledBackupJob {
	return new(ScheduledBackupJob)
}

func (j *ScheduledBackupJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	results, err := j.backups.Run(ctx)
	if err != nil {
		logger.Warning("scheduled backup:", err)
		return
	}
	for _, r := range results {
		if r.Error != "" {
			logger.Warningf("scheduled backup to %s failed: %s", r.Destination, r.Error)
		} else {
			logger.Infof("scheduled backup %s written to %s (%d old removed)", r.Name, r.Destination, r.Pruned)
		}
	}
}
