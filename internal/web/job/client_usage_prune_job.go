package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ClientUsagePruneJob drops per-day usage rows older than service.ClientUsageRetention.
type ClientUsagePruneJob struct {
	clientService service.ClientService
}

func NewClientUsagePruneJob() *ClientUsagePruneJob {
	return new(ClientUsagePruneJob)
}

func (j *ClientUsagePruneJob) Run() {
	removed, err := j.clientService.PruneClientUsageHistory(time.Now())
	if err != nil {
		logger.Warning("client usage: prune old rows:", err)
		return
	}
	if removed > 0 {
		logger.Info("client usage: pruned", removed, "rows older than", service.ClientUsageRetention)
	}
}
