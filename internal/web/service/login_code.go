package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var ErrCodeTooSoon = errors.New("a login code was sent a moment ago, wait before asking for another")

const (
	loginCodeTTL      = 5 * time.Minute
	loginCodeCooldown = 30 * time.Second
	loginCodeAttempts = 5
)

type pendingCode struct {
	hash     [32]byte
	expires  time.Time
	sentAt   time.Time
	attempts int
}

// LoginCodeService issues the one-time codes sent to an admin's Telegram chat.
// Codes live in memory only and belong to one admin each.
type LoginCodeService struct {
	mu      sync.Mutex
	pending map[int]pendingCode
	now     func() time.Time
}

func NewLoginCodeService() *LoginCodeService {
	return &LoginCodeService{pending: map[int]pendingCode{}, now: time.Now}
}

// Issue makes a fresh six-digit code, replacing any earlier one.
func (s *LoginCodeService) Issue(userId int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if prev, ok := s.pending[userId]; ok && now.Sub(prev.sentAt) < loginCodeCooldown {
		return "", ErrCodeTooSoon
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	s.pending[userId] = pendingCode{hash: sha256.Sum256([]byte(code)), expires: now.Add(loginCodeTTL), sentAt: now}
	return code, nil
}

// Verify consumes the code on success; wrong guesses count, and the fifth burns it.
func (s *LoginCodeService) Verify(userId int, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[userId]
	if !ok || s.now().After(p.expires) {
		delete(s.pending, userId)
		return false
	}
	given := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(given[:], p.hash[:]) == 1 {
		delete(s.pending, userId)
		return true
	}
	if p.attempts++; p.attempts >= loginCodeAttempts {
		delete(s.pending, userId)
	} else {
		s.pending[userId] = p
	}
	return false
}
