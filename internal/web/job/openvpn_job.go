package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
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

	deltas, online := mgr.CollectTraffic()
	clientTraffics := make([]*xray.ClientTraffic, 0, len(deltas))
	byTag := map[string]*xray.Traffic{}
	for _, d := range deltas {
		clientTraffics = append(clientTraffics, &xray.ClientTraffic{Email: d.Email, Up: d.Up, Down: d.Down})
		t := byTag[d.Tag]
		if t == nil {
			t = &xray.Traffic{IsInbound: true, Tag: d.Tag}
			byTag[d.Tag] = t
		}
		t.Up += d.Up
		t.Down += d.Down
	}
	traffics := make([]*xray.Traffic, 0, len(byTag))
	for _, t := range byTag {
		traffics = append(traffics, t)
	}
	if len(traffics) > 0 {
		if _, _, err := j.inboundService.AddTraffic(traffics, clientTraffics); err != nil {
			logger.Warning("openvpn job: add traffic:", err)
		}
	}
	j.inboundService.RefreshLocalOnlineClients(online, activeTags)
}
