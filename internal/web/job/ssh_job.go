package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// SSHJob keeps the SSH tunnel servers in line with the database (rebinding a
// listener that died, dropping depleted users) and books their traffic.
type SSHJob struct {
	inboundService service.InboundService
}

func NewSSHJob() *SSHJob {
	return new(SSHJob)
}

func (j *SSHJob) Run() {
	desired, err := j.inboundService.DesiredSSHInstances()
	if err != nil {
		logger.Warning("ssh job: desired instances:", err)
		return
	}
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}
	mgr := sshtunnel.GetManager()
	mgr.Reconcile(desired)

	traffic, online := mgr.CollectTraffic()
	deltas := make([]sidecarDelta, 0, len(traffic))
	for _, t := range traffic {
		deltas = append(deltas, sidecarDelta{Tag: t.Tag, Email: t.Email, Up: t.Up, Down: t.Down})
	}
	bookSidecarTraffic(&j.inboundService, "ssh", deltas, online, activeTags)
}
