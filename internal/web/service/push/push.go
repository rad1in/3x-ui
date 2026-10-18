// Package push delivers event bus notifications to ntfy, Gotify and Slack
// (incoming webhook), each with its own event selection.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

const (
	queueSize = 512
	workers   = 3
)

// Kind names a supported channel.
type Kind string

const (
	Ntfy   Kind = "ntfy"
	Gotify Kind = "gotify"
	Slack  Kind = "slack"
)

// Channel is one configured destination, read fresh for every event.
// URL is the server base for ntfy and Gotify and the full hook for Slack.
type Channel struct {
	Kind    Kind
	Enabled bool
	URL     string
	Topic   string
	Token   string
	Events  map[string]bool
}

// Message is the provider-neutral notification; Priority runs 1 (min) to 5 (max).
type Message struct {
	Title    string
	Body     string
	Priority int
}

type delivery struct {
	ch  Channel
	msg Message
}

// Sender is an event bus subscriber with its own queue, so a slow provider
// retries in the background without holding up other events.
type Sender struct {
	channels func() []Channel
	client   *http.Client
	backoff  []time.Duration
	queue    chan delivery
	quit     chan struct{}
	wg       sync.WaitGroup
	host     string
}

func New(channels func() []Channel) *Sender {
	host, _ := os.Hostname()
	s := &Sender{
		channels: channels,
		client:   &http.Client{Timeout: 10 * time.Second},
		backoff:  []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second},
		queue:    make(chan delivery, queueSize),
		quit:     make(chan struct{}),
		host:     host,
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
	var msg *Message
	for _, ch := range s.channels() {
		if !ch.Enabled || !ch.Events[string(e.Type)] || !ch.configured() {
			continue
		}
		if msg == nil {
			m, ok := Format(e, s.host)
			if !ok {
				return
			}
			msg = &m
		}
		select {
		case s.queue <- delivery{ch: ch, msg: *msg}:
		default:
			logger.Warning("push: queue full, dropped", e.Type, "for", ch.Kind)
		}
	}
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
		retry, err := s.send(d.ch, d.msg)
		if err == nil {
			return
		}
		if !retry || attempt >= len(s.backoff) {
			logger.Warningf("push: %s failed after %d attempt(s): %v", d.ch.Kind, attempt+1, err)
			return
		}
		select {
		case <-s.quit:
			return
		case <-time.After(s.backoff[attempt]):
		}
	}
}

func (c Channel) configured() bool {
	switch c.Kind {
	case Ntfy:
		return c.URL != "" && c.Topic != ""
	case Gotify:
		return c.URL != "" && c.Token != ""
	case Slack:
		return c.URL != ""
	}
	return false
}

// request builds the provider's HTTP call for msg.
func (c Channel) request(msg Message) (*http.Request, error) {
	var (
		url     string
		payload any
		header  = http.Header{}
	)
	switch c.Kind {
	case Ntfy:
		url = strings.TrimRight(c.URL, "/")
		payload = map[string]any{"topic": c.Topic, "title": msg.Title, "message": msg.Body, "priority": msg.Priority}
		if c.Token != "" {
			header.Set("Authorization", "Bearer "+c.Token)
		}
	case Gotify:
		url = strings.TrimRight(c.URL, "/") + "/message"
		payload = map[string]any{"title": msg.Title, "message": msg.Body, "priority": msg.Priority*2 - 1}
		header.Set("X-Gotify-Key", c.Token)
	case Slack:
		url = c.URL
		payload = map[string]string{"text": "*" + slackEscape(msg.Title) + "*\n" + slackEscape(msg.Body)}
	default:
		return nil, fmt.Errorf("unknown channel %q", c.Kind)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "3x-ui-push")
	return req, nil
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func (s *Sender) send(c Channel, msg Message) (retry bool, err error) {
	req, err := c.request(msg)
	if err != nil {
		return false, err
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

// Test sends one message to the channel of the given kind, once, whether or not it is enabled.
func (s *Sender) Test(kind Kind) error {
	for _, ch := range s.channels() {
		if ch.Kind != kind {
			continue
		}
		if !ch.configured() {
			return errors.New("channel is not fully configured")
		}
		_, err := s.send(ch, Message{Title: "3x-ui test", Body: "Test notification from " + s.host, Priority: 3})
		return err
	}
	return fmt.Errorf("unknown channel %q", kind)
}

// Format renders an event as plain text; false means the event carries nothing to send.
func Format(e eventbus.Event, host string) (Message, bool) {
	prefix := "[" + host + "] "
	msg := Message{Priority: 3}
	switch e.Type {
	case eventbus.EventOutboundDown:
		msg.Title, msg.Priority = "Outbound down", 4
	case eventbus.EventOutboundUp:
		msg.Title = "Outbound up"
	case eventbus.EventNodeDown:
		msg.Title, msg.Priority = "Node down", 4
	case eventbus.EventNodeUp:
		msg.Title = "Node up"
	case eventbus.EventPortBlocked:
		msg.Title, msg.Priority = "Port blocked", 4
	case eventbus.EventPortRestored:
		msg.Title = "Port restored"
	case eventbus.EventXrayCrash:
		msg.Title, msg.Priority = "Xray crashed", 5
	case eventbus.EventLoginAttempt:
		msg.Title = "Panel login"
		if d, ok := e.Data.(eventbus.LoginEventData); ok && d.Status == "fail" {
			msg.Priority = 4
		}
	case eventbus.EventWeeklyReport:
		msg.Title = "Weekly summary"
	case eventbus.EventClientCreated:
		msg.Title = "Client created"
	case eventbus.EventClientDeleted:
		msg.Title = "Client deleted"
	case eventbus.EventClientEnabled:
		msg.Title = "Client enabled"
	case eventbus.EventClientDisabled:
		msg.Title = "Client disabled"
	case eventbus.EventClientDepleted:
		msg.Title, msg.Priority = "Client out of traffic", 4
	case eventbus.EventClientExpired:
		msg.Title, msg.Priority = "Client expired", 4
	case eventbus.EventClientRenewed:
		msg.Title = "Client renewed"
	case eventbus.EventClientTrafficReset:
		msg.Title = "Client traffic reset"
	case eventbus.EventClientQuotaWarning:
		msg.Title = "Client near quota"
	case eventbus.EventClientExpiryWarning:
		msg.Title = "Client near expiry"
	default:
		title, lines, ok := eventbus.PlainText(e)
		if !ok {
			return Message{}, false
		}
		return Message{Title: prefix + title, Body: strings.Join(lines, "\n"), Priority: 4}, true
	}
	msg.Title = prefix + msg.Title
	msg.Body = body(e)
	return msg, true
}

func body(e eventbus.Event) string {
	var lines []string
	if e.Source != "" {
		lines = append(lines, e.Source)
	}
	switch d := e.Data.(type) {
	case *eventbus.ClientEventData:
		lines = append(lines, clientLine(*d)...)
	case eventbus.ClientEventData:
		lines = append(lines, clientLine(d)...)
	case *eventbus.OutboundHealthData:
		lines = append(lines, outboundLines(*d)...)
	case eventbus.OutboundHealthData:
		lines = append(lines, outboundLines(d)...)
	case eventbus.LoginEventData:
		lines = append(lines, fmt.Sprintf("%s from %s: %s", d.Username, d.IP, d.Status))
	case *eventbus.NodeHealthData:
		if d.XrayError != "" {
			lines = append(lines, d.XrayError)
		}
	case *eventbus.PortProbeData:
		lines = append(lines, net.JoinHostPort(d.Host, strconv.Itoa(d.Port)))
		if d.Error != "" {
			lines = append(lines, d.Error)
		}
	case *eventbus.WeeklySummaryData:
		return strings.Join(d.Lines(), "\n")
	case string:
		lines = append(lines, d)
	}
	return strings.Join(lines, "\n")
}

func clientLine(d eventbus.ClientEventData) []string {
	used := common.FormatTraffic(d.Used())
	if d.Total > 0 {
		used += " / " + common.FormatTraffic(d.Total)
	}
	lines := []string{"Used: " + used}
	if d.ExpiryTime > 0 {
		lines = append(lines, "Expires: "+d.ExpiryDate())
	}
	return lines
}

func outboundLines(d eventbus.OutboundHealthData) []string {
	var lines []string
	if d.Error != "" {
		lines = append(lines, d.Error)
	}
	if d.Delay > 0 {
		lines = append(lines, fmt.Sprintf("%d ms", d.Delay))
	}
	return lines
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
