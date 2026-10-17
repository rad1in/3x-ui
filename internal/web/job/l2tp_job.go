package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// L2TPJob keeps xl2tpd and its logins in line with the database (restarting
// it after a crash, ending depleted users' links) and books the PPP traffic.
type L2TPJob struct {
	inboundService service.InboundService
	// lastErr keeps a server that cannot run the protocol from logging it every tick.
	lastErr string
}

func NewL2TPJob() *L2TPJob {
	return new(L2TPJob)
}

func (j *L2TPJob) Run() {
	desired, err := j.inboundService.DesiredL2TPInstances()
	if err != nil {
		logger.Warning("l2tp job: desired instances:", err)
		return
	}
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}
	mgr := l2tp.GetManager()
	err = mgr.Reconcile(desired)
	if msg := errString(err); msg != j.lastErr {
		if msg != "" {
			logger.Warning("l2tp job:", msg)
		}
		j.lastErr = msg
	}

	traffic, online := mgr.CollectTraffic()
	deltas := make([]sidecarDelta, 0, len(traffic))
	for _, t := range traffic {
		deltas = append(deltas, sidecarDelta{Tag: t.Tag, Email: t.Email, Up: t.Up, Down: t.Down})
	}
	bookSidecarTraffic(&j.inboundService, "l2tp", deltas, online, activeTags)
}
