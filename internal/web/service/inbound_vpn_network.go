package service

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// vpnClientNetworkKey is where each host-routed VPN keeps the network its
// clients are numbered from; two of them overlapping would fight over routes.
var vpnClientNetworkKey = map[model.Protocol]string{
	model.OpenVPN: "network",
	model.IKEv2:   "pool",
	model.L2TP:    "pool",
	model.SSTP:    "pool",
	model.OCServ:  "pool",
}

// vpnClientNetwork validates the requested client network, or picks the first
// free 10.<second>.N.0/24 when none was asked for.
func (s *InboundService) vpnClientNetwork(label string, selfID int, requested string, second byte) (netip.Prefix, error) {
	protocols := make([]model.Protocol, 0, len(vpnClientNetworkKey))
	for p := range vpnClientNetworkKey {
		protocols = append(protocols, p)
	}
	var others []model.Inbound
	if err := database.GetDB().Where("protocol IN ? AND id <> ?", protocols, selfID).Find(&others).Error; err != nil {
		return netip.Prefix{}, err
	}
	taken := make([]netip.Prefix, 0, len(others))
	for _, ib := range others {
		var parsed map[string]any
		if json.Unmarshal([]byte(ib.Settings), &parsed) != nil {
			continue
		}
		if p, err := netip.ParsePrefix(fmt.Sprint(parsed[vpnClientNetworkKey[ib.Protocol]])); err == nil {
			taken = append(taken, p.Masked())
		}
	}
	overlaps := func(p netip.Prefix) bool {
		for _, t := range taken {
			if t.Overlaps(p) {
				return true
			}
		}
		return false
	}

	requested = strings.TrimSpace(requested)
	if requested == "" || requested == "<nil>" {
		for n := 0; n < 256; n++ {
			p := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, second, byte(n), 0}), 24)
			if !overlaps(p) {
				return p, nil
			}
		}
		return netip.Prefix{}, fmt.Errorf("%s: no free client network left in 10.%d.0.0/16", label, second)
	}
	p, err := netip.ParsePrefix(requested)
	if err != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() || p.Bits() < 16 || p.Bits() > 29 {
		return netip.Prefix{}, fmt.Errorf("%s: client network %q must be a private IPv4 range from /16 to /29", label, requested)
	}
	if overlaps(p.Masked()) {
		return netip.Prefix{}, fmt.Errorf("%s: client network %s overlaps another VPN inbound's", label, p.Masked())
	}
	return p.Masked(), nil
}
