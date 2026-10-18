package sub

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/subsign"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// signingTTL is how long a read of the signing setting is reused, so a request
// costs no query yet a change still applies within seconds.
const signingTTL = 3 * time.Second

type signingState struct {
	mu      sync.Mutex
	loaded  time.Time
	enabled bool
	key     []byte
}

func (s *signingState) current() (bool, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.loaded) < signingTTL {
		return s.enabled, s.key
	}
	cfg, err := (&service.SecurityConfigService{}).Get()
	if err != nil {
		logger.Warning("sub signing: cannot read the setting, links stay unsigned:", err)
		return false, nil
	}
	s.loaded, s.enabled, s.key = time.Now(), cfg.SubSigningEnable && cfg.SubSigningKey != "", []byte(cfg.SubSigningKey)
	return s.enabled, s.key
}

// requireSignature answers 404 to any subscription id request without a valid
// signature while signing is on; short links carry their own secret and pass.
func (a *SUBController) requireSignature(c *gin.Context) {
	subID := c.Param("subid")
	if subID == "" {
		c.Next()
		return
	}
	enabled, key := a.signing.current()
	if !enabled {
		c.Next()
		return
	}
	var exp int64
	if raw := c.Query("exp"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		exp = parsed
	}
	if err := subsign.Verify(key, subID, exp, c.Query("sig"), time.Now()); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Next()
}
