package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// PruneAuditLogJob drops activity-log rows older than service.AuditRetention.
type PruneAuditLogJob struct {
	auditService service.AuditService
}

func NewPruneAuditLogJob() *PruneAuditLogJob {
	return new(PruneAuditLogJob)
}

func (j *PruneAuditLogJob) Run() {
	removed, err := j.auditService.Prune(time.Now().Add(-service.AuditRetention).UnixMilli())
	if err != nil {
		logger.Warning("audit: prune old rows:", err)
		return
	}
	if removed > 0 {
		logger.Info("audit: pruned", removed, "rows older than", service.AuditRetention)
	}
}
