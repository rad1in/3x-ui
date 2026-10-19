package controller

import (
	"strconv"
	"testing"
	"time"
)

// Rotating usernames from one address never trips the per-pair counter, so the
// address as a whole must be cut off.
func TestLoginLimiterBlocksAnAddressRotatingUsernames(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(3, 5*time.Minute, 15*time.Minute)
	limiter.now = func() time.Time { return now }

	var blockedAt int
	for i := range 40 {
		if _, blocked := limiter.registerFailure("198.51.100.7", "user-"+strconv.Itoa(i)); blocked {
			blockedAt = i + 1
			break
		}
	}
	if blockedAt != 3*loginLimitIPFactor {
		t.Fatalf("address blocked at failure %d, want %d", blockedAt, 3*loginLimitIPFactor)
	}
	if _, ok := limiter.allow("198.51.100.7", "a-brand-new-name"); ok {
		t.Fatal("a blocked address was allowed to try another username")
	}
	if _, ok := limiter.allow("198.51.100.8", "user-0"); !ok {
		t.Fatal("blocking one address locked out a different one")
	}
	now = now.Add(16 * time.Minute)
	if _, ok := limiter.allow("198.51.100.7", "a-brand-new-name"); !ok {
		t.Fatal("the address stayed blocked after the cooldown")
	}
}

// Changing the settings applies to the very next attempt, without a restart.
func TestLoginLimiterConfigureChangesTheThreshold(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(5, 5*time.Minute, 15*time.Minute)
	limiter.now = func() time.Time { return now }
	limiter.configure(2, 10*time.Minute)

	limiter.registerFailure("192.0.2.1", "admin")
	until, blocked := limiter.registerFailure("192.0.2.1", "admin")
	if !blocked {
		t.Fatal("the second failure did not block with a limit of two")
	}
	if want := now.Add(10 * time.Minute); !until.Equal(want) {
		t.Fatalf("blocked until %v, want %v", until, want)
	}
}
