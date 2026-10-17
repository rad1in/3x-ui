package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// sidecarDelta is one user's bytes on one inbound, as a VPN sidecar reports them.
type sidecarDelta struct {
	Tag, Email string
	Up, Down   int64
}

// bookSidecarTraffic records a sidecar's deltas per client and per inbound,
// the same way Xray's stats are booked, and refreshes who is online there.
func bookSidecarTraffic(svc *service.InboundService, name string, deltas []sidecarDelta, online, activeTags []string) {
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
		if _, _, err := svc.AddTraffic(traffics, clientTraffics); err != nil {
			logger.Warning(name, "job: add traffic:", err)
		}
	}
	svc.RefreshLocalOnlineClients(online, activeTags)
}
