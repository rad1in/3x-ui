// Package webhook posts panel events (client lifecycle, health, logins) to
// admin-configured URLs, signed with HMAC-SHA256 and retried with backoff, so
// billing bots, n8n or Zapier can act on them.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	queueSize = 1024
	workers   = 4
)

// Config is what the sender reads, fresh, for every event.
type Config struct {
	Enabled bool
	URLs    []string
	Secret  string
	Events  map[string]bool
}

type payload struct {
	ID        string `json:"id"`
	Event     string `json:"event"`
	Timestamp string `json:"timestamp"`
	Server    string `json:"server"`
	Source    string `json:"source,omitempty"`
	Data      any    `json:"data,omitempty"`
}

type delivery struct {
	url, secret, event, id string
	body                   []byte
}

// Sender is an event bus subscriber with its own delivery queue, so a slow
// receiver retries in the background without holding up other events.
type Sender struct {
	config  func() Config
	client  *http.Client
	backoff []time.Duration // waits between attempts; one more attempt than entries
	queue   chan delivery
	quit    chan struct{}
	wg      sync.WaitGroup
	host    string
}

func New(config func() Config) *Sender {
	host, _ := os.Hostname()
	s := &Sender{
		config:  config,
		client:  &http.Client{Timeout: 10 * time.Second},
		backoff: []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second, 2 * time.Minute},
		queue:   make(chan delivery, queueSize),
		quit:    make(chan struct{}),
		host:    host,
	}
	for range workers {
		s.wg.Add(1)
		go s.work()
	}
	return s
}

func (s *Sender) Stop() {
	close(s.quit)
	s.wg.Wait()
}

func (s *Sender) HandleEvent(e eventbus.Event) {
	cfg := s.config()
	if !cfg.Enabled || !cfg.Events[string(e.Type)] || len(cfg.URLs) == 0 {
		return
	}
	id := uuid.NewString()
	body, err := s.encode(id, e)
	if err != nil {
		logger.Warning("webhook: encode", e.Type, err)
		return
	}
	for _, url := range cfg.URLs {
		select {
		case s.queue <- delivery{url: url, secret: cfg.Secret, event: string(e.Type), id: id, body: body}:
		default:
			logger.Warning("webhook: queue full, dropped", e.Type, "for", url)
		}
	}
}

func (s *Sender) encode(id string, e eventbus.Event) ([]byte, error) {
	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return json.Marshal(payload{
		ID: id, Event: string(e.Type), Timestamp: ts.UTC().Format(time.RFC3339),
		Server: s.host, Source: e.Source, Data: e.Data,
	})
}

func (s *Sender) work() {
	defer s.wg.Done()
	for {
		select {
		case <-s.quit:
			return
		case d := <-s.queue:
			s.deliver(d)
		}
	}
}

// deliver retries network errors, 5xx and 429; any other answer is final.
func (s *Sender) deliver(d delivery) {
	for attempt := 0; ; attempt++ {
		retry, err := s.post(d)
		if err == nil {
			return
		}
		if !retry || attempt >= len(s.backoff) {
			logger.Warningf("webhook: %s to %s failed after %d attempt(s): %v", d.event, d.url, attempt+1, err)
			return
		}
		select {
		case <-s.quit:
			return
		case <-time.After(s.backoff[attempt]):
		}
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Sender) post(d delivery) (retry bool, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, d.url, bytes.NewReader(d.body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "3x-ui-webhook")
	req.Header.Set("X-XUI-Event", d.event)
	req.Header.Set("X-XUI-Delivery", d.id)
	if d.secret != "" {
		req.Header.Set("X-XUI-Signature", sign(d.secret, d.body))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return true, err
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return true, fmt.Errorf("HTTP %d", resp.StatusCode)
	default:
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}

// Test sends one webhook.test event to every URL, once, and reports the first failure.
func (s *Sender) Test() error {
	cfg := s.config()
	if len(cfg.URLs) == 0 {
		return errors.New("no webhook URL configured")
	}
	id := uuid.NewString()
	body, err := s.encode(id, eventbus.Event{Type: "webhook.test", Source: "3x-ui"})
	if err != nil {
		return err
	}
	var errs []string
	for _, url := range cfg.URLs {
		if _, err := s.post(delivery{url: url, secret: cfg.Secret, event: "webhook.test", id: id, body: body}); err != nil {
			errs = append(errs, url+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// ParseURLs splits the URL setting, one per line or comma separated.
func ParseURLs(raw string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == ',' || r == '\r' }) {
		if u := strings.TrimSpace(f); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// ParseEvents turns the comma-separated event setting into a lookup set.
func ParseEvents(raw string) map[string]bool {
	out := map[string]bool{}
	for e := range strings.SplitSeq(raw, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out[e] = true
		}
	}
	return out
}
