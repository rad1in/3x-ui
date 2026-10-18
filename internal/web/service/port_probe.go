package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/netsafe"
)

const (
	portProbeTimeout     = 8 * time.Second
	portProbeConcurrency = 16
	portProbeBodyCap     = 64 << 10
	publicHostTTL        = 10 * time.Minute
)

// portProbeProtocols are the inbound protocols that listen on TCP; UDP-only
// ones (hysteria, wireguard, tuic, ...) cannot be checked with a dial.
var portProbeProtocols = map[model.Protocol]bool{
	model.VLESS: true, model.VMESS: true, model.Trojan: true, model.Shadowsocks: true,
	model.MTProto: true, model.HTTP: true, model.Mixed: true,
}

var udpStreamNetworks = map[string]bool{"kcp": true, "quic": true, "hysteria": true}

type portProbeTarget struct {
	InboundId int
	Tag       string
	Host      string
	Port      int
}

type portProbeConfig struct {
	URL    string
	Expect string
}

type portProbeEntry struct {
	fails int
	down  bool
}

// PortProbeService checks that each public TCP inbound port still answers from
// outside and reports a port that stopped (port.blocked) or came back (port.restored).
type PortProbeService struct {
	settings SettingService

	// probe and publicHost are test seams; zero values use the real network.
	probe      func(ctx context.Context, t portProbeTarget) error
	publicHost func() string

	mu        sync.Mutex
	byInbound map[int]*portProbeEntry
	host      string
	hostAt    time.Time
}

func (s *PortProbeService) config() (cfg portProbeConfig, failures int) {
	cfg.URL, _ = s.settings.getString("portProbeUrl")
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Expect, _ = s.settings.getString("portProbeExpect")
	failures, _ = s.settings.getInt("portProbeFailures")
	if failures < 1 {
		failures = 3
	}
	return cfg, failures
}

func (s *PortProbeService) resolvePublicHost() string {
	if h, _ := s.settings.getString("portProbeHost"); strings.TrimSpace(h) != "" {
		return strings.TrimSpace(h)
	}
	if s.publicHost != nil {
		return s.publicHost()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.host == "" || time.Since(s.hostAt) > publicHostTTL {
		if ip := firstPublicIP(publicIPv4Services); ip != "N/A" {
			s.host, s.hostAt = ip, time.Now()
		}
	}
	return s.host
}

// isPublicListen is false for an inbound bound to loopback or a unix socket,
// which an outside probe cannot reach by design (fallback children, local proxies).
func isPublicListen(listen string) bool {
	l := strings.TrimSpace(listen)
	if l == "" || l == "0.0.0.0" || l == "::" {
		return true
	}
	if strings.HasPrefix(l, "/") || strings.HasPrefix(l, "@") || l == "localhost" {
		return false
	}
	ip := net.ParseIP(l)
	return ip == nil || !ip.IsLoopback()
}

func isTCPInbound(ib *model.Inbound) bool {
	if !portProbeProtocols[ib.Protocol] {
		return false
	}
	var stream struct {
		Network string `json:"network"`
	}
	if ib.StreamSettings != "" && json.Unmarshal([]byte(ib.StreamSettings), &stream) == nil {
		return !udpStreamNetworks[stream.Network]
	}
	return true
}

func (s *PortProbeService) targets() ([]portProbeTarget, error) {
	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Where("enable = ?", true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	var nodes []model.Node
	if err := db.Find(&nodes).Error; err != nil {
		return nil, err
	}
	nodeByID := make(map[int]model.Node, len(nodes))
	for _, n := range nodes {
		nodeByID[n.Id] = n
	}

	localHost := ""
	out := make([]portProbeTarget, 0, len(inbounds))
	for i := range inbounds {
		ib := &inbounds[i]
		if ib.Port <= 0 || !isTCPInbound(ib) || !isPublicListen(ib.Listen) {
			continue
		}
		host := ""
		if ib.NodeID != nil {
			n, ok := nodeByID[*ib.NodeID]
			// A node that is down already raised node.down; probing it adds only noise.
			if !ok || n.Status != "online" {
				continue
			}
			host = n.Address
		} else {
			if localHost == "" {
				localHost = s.resolvePublicHost()
			}
			host = localHost
		}
		if host == "" {
			continue
		}
		out = append(out, portProbeTarget{InboundId: ib.Id, Tag: probeLabel(ib), Host: host, Port: ib.Port})
	}
	return out, nil
}

func probeLabel(ib *model.Inbound) string {
	if ib.Remark != "" {
		return ib.Remark
	}
	return ib.Tag
}

// Tick probes every target once and returns the events for the ports whose
// state changed; an outage is reported after the configured run of failures.
func (s *PortProbeService) Tick() []eventbus.Event {
	if on, _ := s.settings.getBool("portProbeEnable"); !on {
		return nil
	}
	targets, err := s.targets()
	if err != nil {
		logger.Warning("port probe: list targets failed:", err)
		return nil
	}
	cfg, threshold := s.config()
	method := "tcp"
	if cfg.URL != "" {
		method = "url"
	}
	check := s.probe
	if check == nil {
		check = func(ctx context.Context, t portProbeTarget) error {
			if cfg.URL != "" {
				return probeByURL(ctx, defaultProbeClient(), cfg, t)
			}
			return probeByTCP(ctx, t)
		}
	}

	errs := make([]error, len(targets))
	sem := make(chan struct{}, portProbeConcurrency)
	var wg sync.WaitGroup
	for i := range targets {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), portProbeTimeout)
			defer cancel()
			errs[i] = check(ctx, targets[i])
		})
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byInbound == nil {
		s.byInbound = map[int]*portProbeEntry{}
	}
	live := make(map[int]bool, len(targets))
	var events []eventbus.Event
	for i, t := range targets {
		live[t.InboundId] = true
		e := s.byInbound[t.InboundId]
		if e == nil {
			e = &portProbeEntry{}
			s.byInbound[t.InboundId] = e
		}
		data := &eventbus.PortProbeData{InboundId: t.InboundId, Tag: t.Tag, Host: t.Host, Port: t.Port, Method: method}
		switch {
		case errs[i] != nil:
			e.fails++
			if e.fails >= threshold && !e.down {
				e.down = true
				data.Error = errs[i].Error()
				events = append(events, eventbus.Event{Type: eventbus.EventPortBlocked, Source: t.Tag, Data: data})
			}
		default:
			if e.down {
				events = append(events, eventbus.Event{Type: eventbus.EventPortRestored, Source: t.Tag, Data: data})
			}
			e.fails, e.down = 0, false
		}
	}
	for id := range s.byInbound {
		if !live[id] {
			delete(s.byInbound, id)
		}
	}
	return events
}

func probeByTCP(ctx context.Context, t portProbeTarget) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)))
	if err != nil {
		return err
	}
	return conn.Close()
}

func defaultProbeClient() *http.Client {
	return &http.Client{
		Timeout:   portProbeTimeout,
		Transport: &http.Transport{DialContext: netsafe.SSRFGuardedDialContext},
	}
}

// probeByURL asks an outside service whether the port is open: the port counts
// as open when the URL answers 2xx and, if Expect is set, the body contains it.
func probeByURL(ctx context.Context, client *http.Client, cfg portProbeConfig, t portProbeTarget) error {
	target := strings.NewReplacer(
		"{host}", url.QueryEscape(t.Host),
		"{port}", strconv.Itoa(t.Port),
	).Replace(cfg.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, portProbeBodyCap))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("probe service answered HTTP %d", resp.StatusCode)
	}
	if cfg.Expect != "" && !strings.Contains(strings.ToLower(string(body)), strings.ToLower(cfg.Expect)) {
		return fmt.Errorf("probe service response lacks %q", cfg.Expect)
	}
	return nil
}
