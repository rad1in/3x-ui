package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

// ClientCleanupJob deletes clients that ran out longer ago than autoDeleteDepletedDays.
type ClientCleanupJob struct {
	cleanup        service.ClientCleanupService
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewClientCleanupJob() *ClientCleanupJob {
	return new(ClientCleanupJob)
}

func (j *ClientCleanupJob) Run() {
	deleted, needRestart, err := j.cleanup.Run(&j.inboundService, time.Now())
	if err != nil {
		logger.Warning("client cleanup:", err)
	}
	if deleted > 0 {
		logger.Infof("client cleanup: deleted %d client(s) out of quota or time for too long", deleted)
		websocket.BroadcastInvalidate(websocket.MessageTypeClients)
	}
	if needRestart {
		j.xrayService.SetToNeedRestart()
	}
}
