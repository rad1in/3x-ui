package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ClientWarningJob publishes the quota and expiry warnings clients have just crossed into.
type ClientWarningJob struct {
	warnings service.ClientWarningService
}

func NewClientWarningJob() *ClientWarningJob {
	return new(ClientWarningJob)
}

func (j *ClientWarningJob) Run() {
	if err := j.warnings.Check(time.Now()); err != nil {
		logger.Warning("client warnings:", err)
	}
}
