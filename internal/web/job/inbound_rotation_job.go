package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

// InboundRotationJob rotates the port/SNI of every inbound whose schedule is due.
type InboundRotationJob struct {
	rotation service.InboundRotationService
	xray     service.XrayService
}

func NewInboundRotationJob() *InboundRotationJob { return &InboundRotationJob{} }

func (j *InboundRotationJob) Run() {
	results := j.rotation.Tick(time.Now())
	if len(results) == 0 {
		return
	}
	for _, r := range results {
		if r.NeedRestart {
			j.xray.SetToNeedRestart()
			break
		}
	}
	websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
}
