package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// IKEv2Job keeps charon in line with the database (restarting it after a
// crash, dropping depleted users) and books the IPsec traffic.
type IKEv2Job struct {
	inboundService service.InboundService
	// lastErr keeps a server that cannot run the protocol from logging it every tick.
	lastErr string
}

func NewIKEv2Job() *IKEv2Job {
	return new(IKEv2Job)
}

func (j *IKEv2Job) Run() {
	desired, err := j.inboundService.DesiredIKEv2Instances()
	if err != nil {
		logger.Warning("ikev2 job: desired instances:", err)
		return
	}
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}
	mgr := ipsec.GetManager()
	err = mgr.Reconcile(desired)
	if msg := errString(err); msg != j.lastErr {
		if msg != "" {
			logger.Warning("ikev2 job:", msg)
		}
		j.lastErr = msg
	}

	traffic, online := mgr.CollectTraffic()
	deltas := make([]sidecarDelta, 0, len(traffic))
	for _, t := range traffic {
		deltas = append(deltas, sidecarDelta{Tag: t.Tag, Email: t.Email, Up: t.Up, Down: t.Down})
	}
	bookSidecarTraffic(&j.inboundService, "ikev2", deltas, online, activeTags)
}
