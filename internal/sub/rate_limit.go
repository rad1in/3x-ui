package sub

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const rateLimitWindow = time.Minute

type rateWindow struct {
	start time.Time
	count int
}

// fixedWindowLimiter allows limit requests per key per minute; windows that
// ended are swept at most once a minute so the map stays bounded.
type fixedWindowLimiter struct {
	limit     int
	now       func() time.Time
	mu        sync.Mutex
	windows   map[string]*rateWindow
	lastSweep time.Time
}

func newFixedWindowLimiter(limit int, now func() time.Time) *fixedWindowLimiter {
	if limit <= 0 {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &fixedWindowLimiter{limit: limit, now: now, windows: map[string]*rateWindow{}}
}

func (l *fixedWindowLimiter) allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) >= rateLimitWindow {
		for k, w := range l.windows {
			if now.Sub(w.start) >= rateLimitWindow {
				delete(l.windows, k)
			}
		}
		l.lastSweep = now
	}
	w := l.windows[key]
	if w == nil || now.Sub(w.start) >= rateLimitWindow {
		l.windows[key] = &rateWindow{start: now, count: 1}
		return true, 0
	}
	if w.count >= l.limit {
		return false, w.start.Add(rateLimitWindow).Sub(now)
	}
	w.count++
	return true, 0
}

func tooManyRequests(c *gin.Context, wait time.Duration) {
	c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	c.AbortWithStatus(http.StatusTooManyRequests)
}

// rateLimit guards every subscription route; a short link counts under its
// token because the subId it aliases is only known inside the handler.
func (a *SUBController) rateLimit(c *gin.Context) {
	if a.ipLimiter != nil {
		if ok, wait := a.ipLimiter.allow(a.subService.VisitorIP(c)); !ok {
			tooManyRequests(c, wait)
			return
		}
	}
	if a.subLimiter != nil {
		key := c.Param("subid")
		if key == "" {
			key = "/s/" + c.Param("token")
		}
		if ok, wait := a.subLimiter.allow(key); !ok {
			tooManyRequests(c, wait)
			return
		}
	}
	c.Next()
}
