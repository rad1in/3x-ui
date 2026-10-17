package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// OpenVPNJob keeps the openvpn servers in line with the database (restarting
// crashed ones, dropping depleted users) and books their traffic.
type OpenVPNJob struct {
	inboundService service.InboundService
}

func NewOpenVPNJob() *OpenVPNJob {
	return new(OpenVPNJob)
}

func (j *OpenVPNJob) Run() {
	desired, err := j.inboundService.DesiredOpenVPNInstances()
	if err != nil {
		logger.Warning("openvpn job: desired instances:", err)
		return
	}
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}
	mgr := openvpn.GetManager()
	mgr.Reconcile(desired)

	traffic, online := mgr.CollectTraffic()
	deltas := make([]sidecarDelta, 0, len(traffic))
	for _, t := range traffic {
		deltas = append(deltas, sidecarDelta{Tag: t.Tag, Email: t.Email, Up: t.Up, Down: t.Down})
	}
	bookSidecarTraffic(&j.inboundService, "openvpn", deltas, online, activeTags)
}
