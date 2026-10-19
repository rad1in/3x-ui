package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// WeeklySummaryJob publishes report.weekly in the configured weekday and hour (panel time).
type WeeklySummaryJob struct {
	summary service.WeeklySummaryService
	loc     *time.Location
}

func NewWeeklySummaryJob(loc *time.Location) *WeeklySummaryJob {
	return &WeeklySummaryJob{loc: loc}
}

func (j *WeeklySummaryJob) Run() {
	now := time.Now().In(j.loc)
	if !j.summary.Due(now) {
		return
	}
	if err := j.summary.Run(now); err != nil {
		logger.Warning("weekly summary:", err)
	}
}
