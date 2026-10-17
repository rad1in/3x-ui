package job

import "github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"

// HostedSponsorsJob runs on a master that hosts panels: republishing the owner's
// sponsors starts scheduled placements on time and carries feed changes over.
type HostedSponsorsJob struct{}

func NewHostedSponsorsJob() *HostedSponsorsJob {
	return new(HostedSponsorsJob)
}

func (j *HostedSponsorsJob) Run() {
	panel.PublishHostedSponsors()
}
