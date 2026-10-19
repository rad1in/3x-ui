package controller

import (
	"strings"
	"sync"
	"time"
)

const (
	loginLimitMaxFailures = 5
	loginLimitWindow      = 5 * time.Minute
	loginLimitCooldown    = 15 * time.Minute
	// Hard ceiling on tracked (ip, username) records. The key includes the
	// caller-supplied username, so an unauthenticated attacker rotating
	// usernames would otherwise grow the map without bound.
	loginLimitMaxRecords = 10000
	// An address as a whole may fail this many times more often than one
	// username on it before it is cut off.
	loginLimitIPFactor = 4
)

var defaultLoginLimiter = newLoginLimiter(loginLimitMaxFailures, loginLimitWindow, loginLimitCooldown)

type loginLimiter struct {
	mu          sync.Mutex
	now         func() time.Time
	maxFailures int
	window      time.Duration
	cooldown    time.Duration
	attempts    map[string]*loginLimitRecord
}

type loginLimitRecord struct {
	failures     []time.Time
	blockedUntil time.Time
}

func newLoginLimiter(maxFailures int, window, cooldown time.Duration) *loginLimiter {
	return &loginLimiter{
		now:         time.Now,
		maxFailures: maxFailures,
		window:      window,
		cooldown:    cooldown,
		attempts:    make(map[string]*loginLimitRecord),
	}
}

// configure applies the operator's settings to the next attempt.
func (l *loginLimiter) configure(maxFailures int, cooldown time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxFailures = maxFailures
	l.cooldown = cooldown
}

// allow refuses while either the (address, username) pair or the address as a
// whole is in cooldown.
func (l *loginLimiter) allow(ip, username string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var latest time.Time
	for _, key := range []string{loginLimitKey(ip, username), loginLimitIPKey(ip)} {
		if until, blocked := l.blockedUntil(key); blocked && until.After(latest) {
			latest = until
		}
	}
	return latest, latest.IsZero()
}

func (l *loginLimiter) blockedUntil(key string) (time.Time, bool) {
	record := l.attempts[key]
	if record == nil {
		return time.Time{}, false
	}
	now := l.now()
	if now.Before(record.blockedUntil) {
		return record.blockedUntil, true
	}
	record.blockedUntil = time.Time{}
	record.failures = pruneLoginFailures(record.failures, now.Add(-l.window))
	if len(record.failures) == 0 {
		delete(l.attempts, key)
	}
	return time.Time{}, false
}

// registerFailure counts a failed login against the pair and against the
// address, which tolerates loginLimitIPFactor times as many failures so a
// shared office address is not shut out by one person's typos.
func (l *loginLimiter) registerFailure(ip, username string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	pairUntil, pairBlocked := l.count(loginLimitKey(ip, username), l.maxFailures, now)
	ipUntil, ipBlocked := l.count(loginLimitIPKey(ip), l.maxFailures*loginLimitIPFactor, now)
	if !pairBlocked && !ipBlocked {
		return time.Time{}, false
	}
	if ipUntil.After(pairUntil) {
		return ipUntil, true
	}
	return pairUntil, true
}

func (l *loginLimiter) count(key string, limit int, now time.Time) (time.Time, bool) {
	record := l.attempts[key]
	if record == nil {
		l.evictForRoom(now)
		record = &loginLimitRecord{}
		l.attempts[key] = record
	}
	record.failures = pruneLoginFailures(record.failures, now.Add(-l.window))
	record.failures = append(record.failures, now)
	if len(record.failures) >= limit {
		record.failures = nil
		record.blockedUntil = now.Add(l.cooldown)
		return record.blockedUntil, true
	}
	return time.Time{}, false
}

func (l *loginLimiter) registerSuccess(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, loginLimitKey(ip, username))
}

// evictForRoom keeps the attempts map bounded before inserting a new record.
// It first reclaims records that are no longer blocked and whose failures have
// aged out of the window; if the map is still at the ceiling (a genuine
// broad flood), it drops one arbitrary record so memory can never grow past the
// cap. Callers hold l.mu.
func (l *loginLimiter) evictForRoom(now time.Time) {
	if len(l.attempts) < loginLimitMaxRecords {
		return
	}
	cutoff := now.Add(-l.window)
	for key, record := range l.attempts {
		if now.Before(record.blockedUntil) {
			continue
		}
		record.failures = pruneLoginFailures(record.failures, cutoff)
		if len(record.failures) == 0 {
			delete(l.attempts, key)
		}
	}
	if len(l.attempts) < loginLimitMaxRecords {
		return
	}
	for key, record := range l.attempts {
		if now.Before(record.blockedUntil) {
			continue
		}
		delete(l.attempts, key)
		return
	}
	for key := range l.attempts {
		delete(l.attempts, key)
		return
	}
}

func loginLimitIPKey(ip string) string {
	return "ip\x00" + strings.TrimSpace(ip)
}

func loginLimitKey(ip, username string) string {
	return strings.TrimSpace(ip) + "\x00" + strings.ToLower(strings.TrimSpace(username))
}

func pruneLoginFailures(failures []time.Time, cutoff time.Time) []time.Time {
	keepFrom := 0
	for keepFrom < len(failures) && failures[keepFrom].Before(cutoff) {
		keepFrom++
	}
	return failures[keepFrom:]
}
