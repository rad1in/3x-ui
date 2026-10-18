package job

import "github.com/mhsanaei/3x-ui/v3/internal/web/service"

// PortProbeJob checks that every public inbound port still answers and
// publishes port.blocked / port.restored on a change.
type PortProbeJob struct {
	probe service.PortProbeService
}

func NewPortProbeJob() *PortProbeJob { return &PortProbeJob{} }

func (j *PortProbeJob) Run() {
	events := j.probe.Tick()
	if EventBus == nil {
		return
	}
	for _, e := range events {
		EventBus.Publish(e)
	}
}
