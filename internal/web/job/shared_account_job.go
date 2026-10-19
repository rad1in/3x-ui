package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// SharedAccountJob flags clients seen from too many addresses or countries.
type SharedAccountJob struct {
	shared service.SharedAccountService
}

func NewSharedAccountJob() *SharedAccountJob {
	return new(SharedAccountJob)
}

func (j *SharedAccountJob) Run() {
	if err := j.shared.Scan(time.Now()); err != nil {
		logger.Warning("shared account scan:", err)
	}
}
