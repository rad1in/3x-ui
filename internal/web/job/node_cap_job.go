package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// NodeCapJob meters each node's monthly traffic and announces caps reached.
type NodeCapJob struct {
	caps service.NodeCapService
}

func NewNodeCapJob() *NodeCapJob { return &NodeCapJob{} }

func (j *NodeCapJob) Run() {
	hits := j.caps.Tick(time.Now())
	if EventBus == nil {
		return
	}
	for _, h := range hits {
		EventBus.Publish(eventbus.Event{
			Type:   eventbus.EventNodeCapReached,
			Source: h.Name,
			Data: &eventbus.NodeCapData{
				NodeId: h.NodeId, UsedBytes: h.UsedBytes, CapBytes: h.CapBytes, Stopped: h.Stopped,
			},
		})
	}
}
