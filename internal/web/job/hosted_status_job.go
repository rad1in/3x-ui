package job

import "github.com/mhsanaei/3x-ui/v3/internal/web/service"

// HostedStatusJob runs only on a hosted panel: it reports usage to the master
// and starts or stops Xray as the owner's limits change.
type HostedStatusJob struct {
	hostedService service.HostedService
	xrayService   service.XrayService
}

func NewHostedStatusJob() *HostedStatusJob {
	return new(HostedStatusJob)
}

func (j *HostedStatusJob) Run() {
	j.hostedService.Reconcile(&j.xrayService)
}
