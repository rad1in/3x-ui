package push

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

type hit struct {
	path   string
	header http.Header
	body   map[string]any
}

type recorder struct {
	mu   sync.Mutex
	hits []hit
	code func(n int) int
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	raw, _ := io.ReadAll(req.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	r.mu.Lock()
	r.hits = append(r.hits, hit{path: req.URL.Path, header: req.Header.Clone(), body: body})
	n := len(r.hits)
	r.mu.Unlock()
	code := http.StatusOK
	if r.code != nil {
		code = r.code(n)
	}
	w.WriteHeader(code)
}

func (r *recorder) wait(t *testing.T, want int) []hit {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.hits)
		out := append([]hit(nil), r.hits...)
		r.mu.Unlock()
		if n >= want {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d requests, want %d", n, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newSender(chs ...Channel) *Sender {
	s := New(func() []Channel { return chs })
	s.backoff = []time.Duration{time.Millisecond, time.Millisecond}
	return s
}

var depleted = eventbus.Event{
	Type: eventbus.EventClientDepleted, Source: "ali",
	Data: &eventbus.ClientEventData{Email: "ali", Up: 1 << 29, Down: 1 << 29, Total: 1 << 30},
}

// ntfy publishes as JSON to the server root so non-ASCII titles survive, and a token becomes a Bearer header.
func TestNtfyPublishShape(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := newSender(Channel{Kind: Ntfy, Enabled: true, URL: srv.URL + "/", Topic: "alerts", Token: "tk_1",
		Events: ParseEvents("client.depleted")})
	defer s.Stop()

	s.HandleEvent(depleted)
	h := rec.wait(t, 1)[0]
	if h.path != "/" || h.body["topic"] != "alerts" || h.body["priority"] != float64(4) {
		t.Fatalf("path=%q body=%v", h.path, h.body)
	}
	if h.header.Get("Authorization") != "Bearer tk_1" {
		t.Fatalf("auth = %q", h.header.Get("Authorization"))
	}
	if h.body["message"] != "ali\nUsed: 1.00GB / 1.00GB" {
		t.Fatalf("message = %q", h.body["message"])
	}
}

// Gotify wants the app token in X-Gotify-Key and its own 0-10 priority scale.
func TestGotifyMessageShape(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := newSender(Channel{Kind: Gotify, Enabled: true, URL: srv.URL, Token: "AppTok",
		Events: ParseEvents("client.depleted")})
	defer s.Stop()

	s.HandleEvent(depleted)
	h := rec.wait(t, 1)[0]
	if h.path != "/message" || h.header.Get("X-Gotify-Key") != "AppTok" || h.body["priority"] != float64(7) {
		t.Fatalf("path=%q key=%q body=%v", h.path, h.header.Get("X-Gotify-Key"), h.body)
	}
}

// Slack mrkdwn treats & < > as control characters, so event text must be escaped.
func TestSlackEscapesMarkup(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s := newSender(Channel{Kind: Slack, Enabled: true, URL: srv.URL + "/services/T/B/x",
		Events: ParseEvents("xray.crash")})
	defer s.Stop()

	s.HandleEvent(eventbus.Event{Type: eventbus.EventXrayCrash, Data: "bad <config> & more"})
	h := rec.wait(t, 1)[0]
	want := "*[" + s.host + "] Xray crashed*\nbad &lt;config&gt; &amp; more"
	if h.path != "/services/T/B/x" || h.body["text"] != want {
		t.Fatalf("path=%q text=%q want %q", h.path, h.body["text"], want)
	}
}

// Only events ticked for a channel reach it, and a disabled channel gets nothing.
func TestEventSelectionPerChannel(t *testing.T) {
	a, b := &recorder{}, &recorder{}
	sa, sb := httptest.NewServer(a), httptest.NewServer(b)
	defer sa.Close()
	defer sb.Close()
	s := newSender(
		Channel{Kind: Slack, Enabled: true, URL: sa.URL, Events: ParseEvents("client.expired")},
		Channel{Kind: Slack, Enabled: false, URL: sb.URL, Events: ParseEvents("client.depleted")},
	)
	defer s.Stop()

	s.HandleEvent(depleted)
	s.HandleEvent(eventbus.Event{Type: eventbus.EventClientExpired, Source: "bo"})
	if got := a.wait(t, 1); got[0].body["text"] == nil || len(got) != 1 {
		t.Fatalf("channel a hits = %v", got)
	}
	time.Sleep(50 * time.Millisecond)
	if len(b.hits) != 0 || len(a.hits) != 1 {
		t.Fatalf("a=%d b=%d", len(a.hits), len(b.hits))
	}
}

// A 503 is retried; a 400 is final.
func TestRetriesServerErrorsOnly(t *testing.T) {
	flaky := &recorder{code: func(n int) int {
		if n == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	}}
	bad := &recorder{code: func(int) int { return http.StatusBadRequest }}
	fs, bs := httptest.NewServer(flaky), httptest.NewServer(bad)
	defer fs.Close()
	defer bs.Close()
	s := newSender(
		Channel{Kind: Slack, Enabled: true, URL: fs.URL, Events: ParseEvents("xray.crash")},
		Channel{Kind: Slack, Enabled: true, URL: bs.URL, Events: ParseEvents("xray.crash")},
	)
	defer s.Stop()

	s.HandleEvent(eventbus.Event{Type: eventbus.EventXrayCrash})
	flaky.wait(t, 2)
	bad.wait(t, 1)
	time.Sleep(50 * time.Millisecond)
	if len(bad.hits) != 1 {
		t.Fatalf("400 was retried: %d hits", len(bad.hits))
	}
}

// The weekly summary reaches a channel as one readable message, not an empty body.
func TestWeeklySummaryIsFormattedForPush(t *testing.T) {
	msg, ok := Format(eventbus.Event{Type: eventbus.EventWeeklyReport, Data: &eventbus.WeeklySummaryData{
		SinceMs: 1760000000000, UntilMs: 1760600000000, Bytes: 3 << 30, TotalClients: 9, NewClients: 2,
		TopUsers: []eventbus.TopUser{{Email: "ali", Bytes: 2 << 30}},
	}}, "h")
	if !ok || msg.Title != "[h] Weekly summary" {
		t.Fatalf("msg = %+v ok=%v", msg, ok)
	}
	for _, want := range []string{"Traffic: 3.00GB", "Clients: 9 (new 2,", "1. ali - 2.00GB"} {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, msg.Body)
		}
	}
}

// A reseller running out of credit is worth a high-priority push with the numbers in the body.
func TestResellerCreditAlertIsFormattedForPush(t *testing.T) {
	msg, ok := Format(eventbus.Event{Type: eventbus.EventResellerCreditLow, Source: "shop", Data: &eventbus.ResellerCreditData{
		Username: "shop", Balance: 950, Threshold: 1000,
	}}, "h")
	if !ok || msg.Title != "[h] Reseller credit is low" || msg.Priority != 4 {
		t.Fatalf("msg = %+v ok=%v", msg, ok)
	}
	if want := "Reseller shop is low on credit: 950 left, alert level 1000"; msg.Body != want {
		t.Fatalf("body = %q, want %q", msg.Body, want)
	}
}

// The test button reports the provider's refusal and works while the channel is switched off.
func TestTestReportsProviderErrorAndIgnoresEnabled(t *testing.T) {
	ok := &recorder{}
	rejecting := &recorder{code: func(int) int { return http.StatusForbidden }}
	os, rs := httptest.NewServer(ok), httptest.NewServer(rejecting)
	defer os.Close()
	defer rs.Close()

	s := newSender(Channel{Kind: Gotify, Enabled: false, URL: os.URL, Token: "t"})
	defer s.Stop()
	if err := s.Test(Gotify); err != nil {
		t.Fatal(err)
	}
	ok.wait(t, 1)

	s2 := newSender(Channel{Kind: Slack, URL: rs.URL})
	defer s2.Stop()
	if err := s2.Test(Slack); err == nil || err.Error() != "HTTP 403" {
		t.Fatalf("err = %v", err)
	}
	if err := s2.Test(Ntfy); err == nil {
		t.Fatal("unconfigured kind must fail")
	}
}
