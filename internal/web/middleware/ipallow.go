package middleware

import (
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// The cache spares a settings read per request; a save invalidates it at once,
// and the TTL covers a change made by another process (CLI, import).
const ipAllowlistTTL = 5 * time.Second

var ipAllowCache struct {
	mu     sync.Mutex
	loaded time.Time
	prefix []netip.Prefix
}

// InvalidateIPAllowlist drops the cached list so the next request re-reads it.
func InvalidateIPAllowlist() {
	ipAllowCache.mu.Lock()
	ipAllowCache.loaded = time.Time{}
	ipAllowCache.mu.Unlock()
}

// parseAllowlist skips entries it cannot read: the save path validates them,
// and a stale bad value must not lock everybody out.
func parseAllowlist(raw string) []netip.Prefix {
	var out []netip.Prefix
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			addr = addr.Unmap()
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		} else if p, err := netip.ParsePrefix(entry); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

func prefixesContain(prefixes []netip.Prefix, ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IPAllowlistContains reports whether ip passes the raw comma-separated list;
// an empty list admits everyone.
func IPAllowlistContains(raw, ip string) bool {
	prefixes := parseAllowlist(raw)
	return len(prefixes) == 0 || prefixesContain(prefixes, ip)
}

func currentAllowlist(load func() string) []netip.Prefix {
	ipAllowCache.mu.Lock()
	defer ipAllowCache.mu.Unlock()
	if time.Since(ipAllowCache.loaded) > ipAllowlistTTL {
		ipAllowCache.prefix = parseAllowlist(load())
		ipAllowCache.loaded = time.Now()
	}
	return ipAllowCache.prefix
}

// IPAllowlistMiddleware answers clients outside the panel allowlist with
// reject (the decoy site, else 404). An empty list turns it off.
func IPAllowlistMiddleware(load func() string, clientIP func(*gin.Context) string, reject gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		prefixes := currentAllowlist(load)
		if len(prefixes) == 0 || prefixesContain(prefixes, clientIP(c)) {
			c.Next()
			return
		}
		if reject != nil {
			reject(c)
		}
		c.Abort()
	}
}
