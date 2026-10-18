package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
)

var openvpnDefaultDNS = []any{"1.1.1.1", "8.8.8.8"}

// normalizeOpenVPNSettings gives an openvpn inbound everything it needs to start:
// a transport, its own certificates (kept across edits), a client network no
// other VPN inbound routes, DNS servers and a password for every client.
func (s *InboundService) normalizeOpenVPNSettings(inbound *model.Inbound, oldSettings string) error {
	if inbound.Protocol != model.OpenVPN {
		return nil
	}
	// Tunnel devices and client networks are host-wide, shared with the owner's panel.
	if hosted.Enabled() {
		return fmt.Errorf("%w: openvpn", ErrHostedForbidden)
	}
	parsed := map[string]any{}
	if t := strings.TrimSpace(inbound.Settings); t != "" && t != "null" {
		if err := json.Unmarshal([]byte(t), &parsed); err != nil || parsed == nil {
			return fmt.Errorf("openvpn: invalid settings: %v", err)
		}
	}
	if parsed["proto"] != "tcp" {
		parsed["proto"] = "udp"
	}

	pki := openvpnPKIOf(parsed["pki"])
	if !pki.Complete() {
		var old map[string]any
		if json.Unmarshal([]byte(oldSettings), &old) == nil {
			pki = openvpnPKIOf(old["pki"])
		}
	}
	if !pki.Complete() {
		var err error
		if pki, err = openvpn.NewPKI("x-ui openvpn"); err != nil {
			return err
		}
	}
	parsed["pki"] = pki

	network, err := s.vpnClientNetwork("openvpn", inbound.Id, fmt.Sprint(parsed["network"]), 88)
	if err != nil {
		return err
	}
	parsed["network"] = network.String()

	dns, _ := parsed["dns"].([]any)
	if len(dns) == 0 {
		dns = openvpnDefaultDNS
	}
	for _, d := range dns {
		if _, err := netip.ParseAddr(fmt.Sprint(d)); err != nil {
			return fmt.Errorf("openvpn: dns %q is not an IP address", d)
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

func openvpnPKIOf(v any) openvpn.PKI {
	var pki openvpn.PKI
	if bs, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(bs, &pki)
	}
	return pki
}

// DesiredOpenVPNInstances is every local openvpn inbound with the clients it
// may serve now: disabled and depleted accounts are left out, so they are refused.
func (s *InboundService) DesiredOpenVPNInstances() ([]openvpn.Instance, error) {
	if hostedSidecarsHalted() {
		return nil, nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.OpenVPN, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	instances := make([]openvpn.Instance, 0, len(inbounds))
	var emails []string
	for _, ib := range inbounds {
		if inst, ok := openvpn.InstanceFromInbound(ib); ok {
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

// applyLocalOpenVPN hands a client edit to the running server at once; the
// openvpn job reconciles anything this misses.
func (s *InboundService) applyLocalOpenVPN(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.OpenVPN || inbound.NodeID != nil {
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
		logger.Debug("openvpn: immediate client apply failed for inbound", inboundId, ":", err)
	}
}

// applyLocalSidecar re-applies a protocol served outside Xray after a client
// change; it reports false for Xray protocols, which take per-user API calls.
func (s *InboundService) applyLocalSidecar(protocol model.Protocol, inboundId int) bool {
	switch protocol {
	case model.MTProto:
		s.applyLocalMtproto(inboundId)
	case model.AmneziaWG:
		s.applyLocalAmneziaWG(inboundId)
	case model.TUIC:
		s.applyLocalTuic(inboundId)
	case model.OpenVPN:
		s.applyLocalOpenVPN(inboundId)
	case model.IKEv2:
		s.applyLocalIKEv2(inboundId)
	case model.L2TP:
		s.applyLocalL2TP(inboundId)
	case model.SSH:
		s.applyLocalSSH(inboundId)
	case model.SSTP:
		s.applyLocalSSTP(inboundId)
	default:
		return false
	}
	return true
}

// OpenVPNProfile is the .ovpn for an inbound, pointing at its share address
// or, failing that, the host the admin reached the panel on.
func (s *InboundService) OpenVPNProfile(id int, requestHost string) (name, body string, err error) {
	ib, err := s.GetInbound(id)
	if err != nil {
		return "", "", err
	}
	inst, _, ok := openvpn.ServerOf(ib)
	if !ok {
		return "", "", fmt.Errorf("inbound %d is not a configured openvpn inbound", id)
	}
	host := requestHost
	if addr := strings.TrimSpace(ib.ShareAddr); ib.ShareAddrStrategy == "custom" && addr != "" {
		host = addr
	} else if l := strings.TrimSpace(ib.Listen); l != "" && l != "0.0.0.0" && l != "::" {
		host = l
	}
	remark := ib.Remark
	if remark == "" {
		remark = ib.Tag
	}
	return fmt.Sprintf("openvpn-%d.ovpn", ib.Id), openvpn.ClientProfile(inst, host, remark), nil
}
