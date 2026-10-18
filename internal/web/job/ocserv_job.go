package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// OCServJob keeps the ocserv servers in line with the database (restarting
// crashed ones, ending depleted users' sessions) and books their traffic.
type OCServJob struct {
	inboundService service.InboundService
	// lastErr keeps a host that cannot run ocserv from logging it every tick.
	lastErr string
}

func NewOCServJob() *OCServJob {
	return new(OCServJob)
}

func (j *OCServJob) Run() {
	desired, err := j.inboundService.DesiredOCServInstances()
	if err != nil {
		logger.Warning("ocserv job: desired instances:", err)
		return
	}
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}
	mgr := ocserv.GetManager()
	err = mgr.Reconcile(desired)
	if msg := errString(err); msg != j.lastErr {
		if msg != "" {
			logger.Warning("ocserv job:", msg)
		}
		j.lastErr = msg
	}

	traffic, online := mgr.CollectTraffic()
	deltas := make([]sidecarDelta, 0, len(traffic))
	for _, t := range traffic {
		deltas = append(deltas, sidecarDelta{Tag: t.Tag, Email: t.Email, Up: t.Up, Down: t.Down})
	}
	bookSidecarTraffic(&j.inboundService, "ocserv", deltas, online, activeTags)
}
