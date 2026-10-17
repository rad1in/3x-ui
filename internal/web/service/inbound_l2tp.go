package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// l2tpPort is fixed: L2TP clients only ever dial UDP 1701.
const l2tpPort = 1701

// newPSK is typed by hand on phones, so it avoids look-alike characters.
func newPSK() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 20)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// normalizeL2TPSettings gives an l2tp inbound everything it needs to start:
// a pre-shared key (kept across edits), a client pool no other VPN inbound
// routes, DNS servers and a password for every client.
func (s *InboundService) normalizeL2TPSettings(inbound *model.Inbound, oldSettings string) error {
	if inbound.Protocol != model.L2TP {
		return nil
	}
	if hosted.Enabled() {
		return fmt.Errorf("%w: l2tp", ErrHostedForbidden)
	}
	inbound.Port = l2tpPort
	parsed := map[string]any{}
	if t := strings.TrimSpace(inbound.Settings); t != "" && t != "null" {
		if err := json.Unmarshal([]byte(t), &parsed); err != nil || parsed == nil {
			return fmt.Errorf("l2tp: invalid settings: %v", err)
		}
	}
	psk, _ := parsed["psk"].(string)
	psk = strings.TrimSpace(psk)
	if psk == "" {
		var old map[string]any
		if json.Unmarshal([]byte(oldSettings), &old) == nil {
			psk, _ = old["psk"].(string)
		}
	}
	if psk == "" {
		var err error
		if psk, err = newPSK(); err != nil {
			return err
		}
	}
	parsed["psk"] = psk

	pool, err := s.vpnClientNetwork("l2tp", inbound.Id, fmt.Sprint(parsed["pool"]), 90)
	if err != nil {
		return err
	}
	parsed["pool"] = pool.String()

	dns, _ := parsed["dns"].([]any)
	if len(dns) == 0 {
		dns = openvpnDefaultDNS
	}
	for _, d := range dns {
		if _, err := netip.ParseAddr(fmt.Sprint(d)); err != nil {
			return fmt.Errorf("l2tp: dns %q is not an IP address", d)
		}
	}
	parsed["dns"] = dns

	clients, _ := parsed["clients"].([]any)
	for _, c := range clients {
		if m, ok := c.(map[string]any); ok {
			if p, _ := m["password"].(string); strings.TrimSpace(p) == "" {
				m["password"] = strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
			}
		}
	}
	if clients == nil {
		clients = []any{}
	}
	parsed["clients"] = clients

	bs, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return err
	}
	inbound.Settings = string(bs)
	return nil
}

// DesiredL2TPInstances is every local l2tp inbound with the clients it may
// serve now: disabled and depleted accounts are left out, so they are refused.
func (s *InboundService) DesiredL2TPInstances() ([]l2tp.Instance, error) {
	if hostedSidecarsHalted() {
		return nil, nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.L2TP, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	instances := make([]l2tp.Instance, 0, len(inbounds))
	var emails []string
	for _, ib := range inbounds {
		if inst, ok := l2tp.InstanceFromInbound(ib); ok {
			instances = append(instances, inst)
			for email := range inst.Users {
				emails = append(emails, email)
			}
		}
	}
	disabled, err := trafficDisabledEmails(db, emails)
	if err != nil {
		return nil, err
	}
	served := instances[:0]
	for _, inst := range instances {
		for email := range inst.Users {
			if _, off := disabled[email]; off {
				delete(inst.Users, email)
			}
		}
		if len(inst.Users) > 0 {
			served = append(served, inst)
		}
	}
	return served, nil
}

// applyLocalL2TP hands a client edit to xl2tpd's logins at once; the l2tp job
// reconciles anything this misses.
func (s *InboundService) applyLocalL2TP(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.L2TP || inbound.NodeID != nil {
		return
	}
	rt, err := s.runtimeFor(inbound)
	if err != nil {
		return
	}
	payload := inbound
	if inbound.Enable {
		if built, bErr := s.buildInboundForLocalRuntime(database.GetDB(), inbound); bErr == nil {
			payload = built
		}
	}
	if err := rt.UpdateInbound(context.Background(), inbound, payload); err != nil {
		logger.Debug("l2tp: immediate client apply failed for inbound", inboundId, ":", err)
	}
}
