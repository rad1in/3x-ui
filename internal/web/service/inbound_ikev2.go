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
)

// ikev2Port is fixed: native clients dial UDP 500 and float to 4500.
const ikev2Port = 500

// normalizeIKEv2Settings gives an ikev2 inbound everything it needs to start:
// certificates naming its address (the CA kept across edits), a client pool no
// other VPN inbound routes, DNS servers and a password for every client.
func (s *InboundService) normalizeIKEv2Settings(inbound *model.Inbound, oldSettings string) error {
	if inbound.Protocol != model.IKEv2 {
		return nil
	}
	if hosted.Enabled() {
		return fmt.Errorf("%w: ikev2", ErrHostedForbidden)
	}
	inbound.Port = ikev2Port
	parsed := map[string]any{}
	if t := strings.TrimSpace(inbound.Settings); t != "" && t != "null" {
		if err := json.Unmarshal([]byte(t), &parsed); err != nil || parsed == nil {
			return fmt.Errorf("ikev2: invalid settings: %v", err)
		}
	}
	address, _ := parsed["serverAddress"].(string)
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("ikev2: set the server address (domain or IP) clients connect to")
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

	pool, err := s.vpnClientNetwork("ikev2", inbound.Id, fmt.Sprint(parsed["pool"]), 89)
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
			return fmt.Errorf("ikev2: dns %q is not an IP address", d)
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

func ikev2PKIOf(v any) ipsec.PKI {
	var pki ipsec.PKI
	if bs, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(bs, &pki)
	}
	return pki
}

// DesiredIKEv2Instances is every local ikev2 inbound with the clients it may
// serve now: disabled and depleted accounts are left out, so they are refused.
func (s *InboundService) DesiredIKEv2Instances() ([]ipsec.Instance, error) {
	if hostedSidecarsHalted() {
		return nil, nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.IKEv2, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	instances := make([]ipsec.Instance, 0, len(inbounds))
	var emails []string
	for _, ib := range inbounds {
		if inst, ok := ipsec.InstanceFromInbound(ib); ok {
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

// applyLocalIKEv2 hands a client edit to charon at once; the ikev2 job
// reconciles anything this misses.
func (s *InboundService) applyLocalIKEv2(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.IKEv2 || inbound.NodeID != nil {
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
		logger.Debug("ikev2: immediate client apply failed for inbound", inboundId, ":", err)
	}
}

// IKEv2 profile formats a client can download.
const (
	IKEv2ProfileApple   = "mobileconfig"
	IKEv2ProfileAndroid = "sswan"
	IKEv2ProfileCA      = "ca"
)

// IKEv2Profile renders one client's profile of an ikev2 inbound: the name to
// save it under, its media type and body. A non-empty dial is the address
// clients connect to when a host moves it away from the certificate's name.
func IKEv2Profile(ib *model.Inbound, email, format, dial string) (name, mime string, body []byte, err error) {
	inst, clients, ok := ipsec.ServerOf(ib)
	if !ok {
		return "", "", nil, fmt.Errorf("inbound %d is not a configured ikev2 inbound", ib.Id)
	}
	inst.Dial = dial
	var client *ipsec.Client
	for i := range clients {
		if clients[i].Email == email {
			client = &clients[i]
			break
		}
	}
	if client == nil {
		return "", "", nil, fmt.Errorf("inbound %d has no client %q", ib.Id, email)
	}
	remark := ib.Remark
	if remark == "" {
		remark = ib.Tag
	}
	base := fmt.Sprintf("ikev2-%d", ib.Id)
	switch format {
	case IKEv2ProfileApple:
		return base + ".mobileconfig", "application/x-apple-aspen-config", ipsec.MobileConfig(inst, client.Email, client.Password, remark), nil
	case IKEv2ProfileAndroid:
		return base + ".sswan", "application/vnd.strongswan.profile", ipsec.SSWAN(inst, client.Email, remark), nil
	case IKEv2ProfileCA:
		return base + "-ca.cer", "application/pkix-cert", ipsec.CADER(inst), nil
	}
	return "", "", nil, fmt.Errorf("unknown ikev2 profile format %q", format)
}

// IKEv2Profile is IKEv2Profile for an inbound of this panel, by id.
func (s *InboundService) IKEv2Profile(id int, email, format string) (string, string, []byte, error) {
	ib, err := s.GetInbound(id)
	if err != nil {
		return "", "", nil, err
	}
	return IKEv2Profile(ib, email, format, "")
}
