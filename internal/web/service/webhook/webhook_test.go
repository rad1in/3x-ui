package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

type received struct {
	mu     sync.Mutex
	bodies [][]byte
	sigs   []string
	events []string
}

func (r *received) handler(status func(n int) int) http.HandlerFunc {
	var n atomic.Int32
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.sigs = append(r.sigs, req.Header.Get("X-XUI-Signature"))
		r.events = append(r.events, req.Header.Get("X-XUI-Event"))
		r.mu.Unlock()
		w.WriteHeader(status(int(n.Add(1))))
	}
}

func (r *received) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

func waitCount(t *testing.T, r *received, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for r.count() < want {
		if time.Now().After(deadline) {
			t.Fatalf("got %d deliveries, want %d", r.count(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newTestSender(cfg Config) *Sender {
	s := New(func() Config { return cfg })
	s.backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	return s
}

func clientEvent(t eventbus.EventType) eventbus.Event {
	return eventbus.Event{Type: t, Source: "ali", Timestamp: time.Unix(1700000000, 0),
		Data: &eventbus.ClientEventData{Email: "ali", SubID: "s1", Total: 1 << 30, Up: 100, Down: 200}}
}

// A receiver checks the body against the shared secret before trusting it.
func TestDeliversSignedEventsToEveryURL(t *testing.T) {
	var a, b received
	ok := func(int) int { return http.StatusOK }
	srvA, srvB := httptest.NewServer(a.handler(ok)), httptest.NewServer(b.handler(ok))
	defer srvA.Close()
	defer srvB.Close()
	s := newTestSender(Config{Enabled: true, URLs: []string{srvA.URL, srvB.URL}, Secret: "s3cret",
		Events: map[string]bool{"client.depleted": true}})
	defer s.Stop()

	s.HandleEvent(clientEvent(eventbus.EventClientDepleted))
	waitCount(t, &a, 1)
	waitCount(t, &b, 1)

	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(a.bodies[0])
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); a.sigs[0] != want {
		t.Fatalf("signature = %q, want %q", a.sigs[0], want)
	}
	var p struct {
		Event string `json:"event"`
		Data  struct {
			Email string `json:"email"`
			Total int64  `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(a.bodies[0], &p); err != nil {
		t.Fatal(err)
	}
	if p.Event != "client.depleted" || p.Data.Email != "ali" || p.Data.Total != 1<<30 || a.events[0] != "client.depleted" {
		t.Fatalf("payload = %s, event header %q", a.bodies[0], a.events[0])
	}
}

func TestSkipsUnselectedEventsAndDisabledConfig(t *testing.T) {
	var r received
	srv := httptest.NewServer(r.handler(func(int) int { return http.StatusOK }))
	defer srv.Close()
	on := newTestSender(Config{Enabled: true, URLs: []string{srv.URL}, Events: map[string]bool{"client.created": true}})
	defer on.Stop()
	off := newTestSender(Config{Enabled: false, URLs: []string{srv.URL}, Events: map[string]bool{"client.deleted": true}})
	defer off.Stop()

	on.HandleEvent(clientEvent(eventbus.EventClientDeleted))
	off.HandleEvent(clientEvent(eventbus.EventClientDeleted))
	on.HandleEvent(clientEvent(eventbus.EventClientCreated))
	waitCount(t, &r, 1)
	time.Sleep(50 * time.Millisecond)
	if r.count() != 1 || r.events[0] != "client.created" {
		t.Fatalf("delivered %v; only the selected event of the enabled sender should go out", r.events)
	}
}

func TestRetriesServerErrorsUntilAccepted(t *testing.T) {
	var r received
	srv := httptest.NewServer(r.handler(func(n int) int {
		if n == 1 {
			return http.StatusTooManyRequests
		}
		if n < 3 {
			return http.StatusBadGateway
		}
		return http.StatusOK
	}))
	defer srv.Close()
	s := newTestSender(Config{Enabled: true, URLs: []string{srv.URL}, Events: map[string]bool{"client.expired": true}})
	defer s.Stop()
	s.HandleEvent(clientEvent(eventbus.EventClientExpired))
	waitCount(t, &r, 3)
	time.Sleep(50 * time.Millisecond)
	if r.count() != 3 {
		t.Fatalf("attempts = %d; want two retries and then stop at the 200", r.count())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if string(r.bodies[0]) != string(r.bodies[2]) {
		t.Fatal("a retry must resend the same delivery (same id), not a new one")
	}
}

// A 4xx means the receiver rejected this delivery; sending it again cannot help.
func TestDoesNotRetryClientErrors(t *testing.T) {
	var r received
	srv := httptest.NewServer(r.handler(func(int) int { return http.StatusUnauthorized }))
	defer srv.Close()
	s := newTestSender(Config{Enabled: true, URLs: []string{srv.URL}, Events: map[string]bool{"client.expired": true}})
	defer s.Stop()
	s.HandleEvent(clientEvent(eventbus.EventClientExpired))
	waitCount(t, &r, 1)
	time.Sleep(50 * time.Millisecond)
	if r.count() != 1 {
		t.Fatalf("attempts = %d, want 1", r.count())
	}
}

func TestGivesUpAfterTheLastAttempt(t *testing.T) {
	var r received
	srv := httptest.NewServer(r.handler(func(int) int { return http.StatusInternalServerError }))
	defer srv.Close()
	s := newTestSender(Config{Enabled: true, URLs: []string{srv.URL}, Events: map[string]bool{"client.expired": true}})
	defer s.Stop()
	s.HandleEvent(clientEvent(eventbus.EventClientExpired))
	waitCount(t, &r, len(s.backoff)+1)
	time.Sleep(50 * time.Millisecond)
	if r.count() != len(s.backoff)+1 {
		t.Fatalf("attempts = %d, want %d", r.count(), len(s.backoff)+1)
	}
}
