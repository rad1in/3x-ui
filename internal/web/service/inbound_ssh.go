package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
)

// normalizeSSHSettings gives an ssh inbound everything it needs to start: its
// own host key (kept across edits, so clients do not see a changed server)
// and a password for every client.
func (s *InboundService) normalizeSSHSettings(inbound *model.Inbound, oldSettings string) error {
	if inbound.Protocol != model.SSH {
		return nil
	}
	if inbound.Port <= 0 {
		return fmt.Errorf("ssh: a port is required")
	}
	parsed := map[string]any{}
	if t := strings.TrimSpace(inbound.Settings); t != "" && t != "null" {
		if err := json.Unmarshal([]byte(t), &parsed); err != nil || parsed == nil {
			return fmt.Errorf("ssh: invalid settings: %v", err)
		}
	}
	key, _ := parsed["hostKey"].(string)
	if sshtunnel.Fingerprint(key) == "" {
		key = ""
		var old map[string]any
		if json.Unmarshal([]byte(oldSettings), &old) == nil {
			key, _ = old["hostKey"].(string)
		}
	}
	if sshtunnel.Fingerprint(key) == "" {
		var err error
		if key, err = sshtunnel.NewHostKey(); err != nil {
			return err
		}
	}
	parsed["hostKey"] = key
	if b, ok := parsed["banner"].(string); ok {
		parsed["banner"] = strings.TrimSpace(b)
	} else {
		parsed["banner"] = ""
	}
	if _, ok := parsed["allowLocal"].(bool); !ok {
		parsed["allowLocal"] = false
	}

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

// DesiredSSHInstances is every local ssh inbound with the clients it may
// serve now: disabled and depleted accounts are left out, so they are refused.
func (s *InboundService) DesiredSSHInstances() ([]sshtunnel.Instance, error) {
	if hostedSidecarsHalted() {
		return nil, nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.SSH, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	instances := make([]sshtunnel.Instance, 0, len(inbounds))
	var emails []string
	for _, ib := range inbounds {
		if inst, ok := sshtunnel.InstanceFromInbound(ib); ok {
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

// applyLocalSSH hands a client edit to the running server at once; the ssh
// job reconciles anything this misses.
func (s *InboundService) applyLocalSSH(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.SSH || inbound.NodeID != nil {
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
		logger.Debug("ssh: immediate client apply failed for inbound", inboundId, ":", err)
	}
}
