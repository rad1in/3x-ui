package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
)

// sstpDefaultPort is the one Windows' built-in client dials.
const sstpDefaultPort = 443

// normalizeSSTPSettings gives an sstp inbound everything it needs to start:
// a certificate naming its address (the CA kept across edits), a client pool
// no other VPN inbound routes, DNS servers and a password for every client.
func (s *InboundService) normalizeSSTPSettings(inbound *model.Inbound, oldSettings string) error {
	if inbound.Protocol != model.SSTP {
		return nil
	}
	// The client network and the host's pppd secrets are shared with the owner's panel.
	if hosted.Enabled() {
		return fmt.Errorf("%w: sstp", ErrHostedForbidden)
	}
	if inbound.Port <= 0 {
		inbound.Port = sstpDefaultPort
	}
	parsed := map[string]any{}
	if t := strings.TrimSpace(inbound.Settings); t != "" && t != "null" {
		if err := json.Unmarshal([]byte(t), &parsed); err != nil || parsed == nil {
			return fmt.Errorf("sstp: invalid settings: %v", err)
		}
	}
	address, _ := parsed["serverAddress"].(string)
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("sstp: set the server address (domain or IP) clients connect to")
	}
	parsed["serverAddress"] = address

	pki := ikev2PKIOf(parsed["pki"])
	if !pki.Complete() {
		var old map[string]any
		if json.Unmarshal([]byte(oldSettings), &old) == nil {
			pki = ikev2PKIOf(old["pki"])
		}
	}
	pki, err := ipsec.PKIFor(pki, address)
	if err != nil {
		return err
	}
	parsed["pki"] = pki

	pool, err := s.vpnClientNetwork("sstp", inbound.Id, fmt.Sprint(parsed["pool"]), 91)
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
			return fmt.Errorf("sstp: dns %q is not an IP address", d)
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

// DesiredSSTPInstances is every local sstp inbound with the clients it may
// serve now: disabled and depleted accounts are left out, so they are refused.
func (s *InboundService) DesiredSSTPInstances() ([]sstp.Instance, error) {
	if hostedSidecarsHalted() {
		return nil, nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.SSTP, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	instances := make([]sstp.Instance, 0, len(inbounds))
	var emails []string
	for _, ib := range inbounds {
		if inst, ok := sstp.InstanceFromInbound(ib); ok {
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

// applyLocalSSTP hands a client edit to the running listener at once; the sstp
// job reconciles anything this misses.
func (s *InboundService) applyLocalSSTP(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.SSTP || inbound.NodeID != nil {
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
		logger.Debug("sstp: immediate client apply failed for inbound", inboundId, ":", err)
	}
}

// SSTPCA is the certificate Windows must trust to connect to an sstp inbound:
// the file name to save it under, its media type and the DER body.
func SSTPCA(ib *model.Inbound) (name, mime string, body []byte, err error) {
	inst, _, ok := sstp.ServerOf(ib)
	if !ok {
		return "", "", nil, fmt.Errorf("inbound %d is not a configured sstp inbound", ib.Id)
	}
	return fmt.Sprintf("sstp-%d-ca.cer", ib.Id), "application/pkix-cert", ipsec.CADER(ipsec.Instance{PKI: inst.PKI}), nil
}

// SSTPCA is SSTPCA for an inbound of this panel, by id.
func (s *InboundService) SSTPCA(id int) (string, string, []byte, error) {
	ib, err := s.GetInbound(id)
	if err != nil {
		return "", "", nil, err
	}
	return SSTPCA(ib)
}
