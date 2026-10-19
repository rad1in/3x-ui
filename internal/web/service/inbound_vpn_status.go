package service

import (
	"sort"

	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
)

// VPNStatus is whether a VPN inbound served outside Xray is up and, if not,
// why: a missing binary, a kernel without PPP or IPsec, a clashing daemon.
type VPNStatus struct {
	Id      int    `json:"id" example:"4"`
	Running bool   `json:"running" example:"false"`
	Reason  string `json:"reason,omitempty" example:"this kernel has no PPP support (/dev/ppp is missing), which L2TP needs"`
}

// VPNStatuses reports the OpenVPN, IKEv2, L2TP, SSH, SSTP and ocserv inbounds that should be
// serving someone now; idle ones are left out since nothing runs for them.
func (s *InboundService) VPNStatuses() ([]VPNStatus, error) {
	out := []VPNStatus{}
	ovpn, err := s.DesiredOpenVPNInstances()
	if err != nil {
		return nil, err
	}
	for _, inst := range ovpn {
		running, reason := openvpn.GetManager().Status(inst.Id)
		out = append(out, VPNStatus{Id: inst.Id, Running: running, Reason: reasonIfDown(running, reason)})
	}
	ike, err := s.DesiredIKEv2Instances()
	if err != nil {
		return nil, err
	}
	if len(ike) > 0 {
		running, reason := ipsec.GetManager().Status()
		for _, inst := range ike {
			out = append(out, VPNStatus{Id: inst.Id, Running: running, Reason: reasonIfDown(running, reason)})
		}
	}
	l2, err := s.DesiredL2TPInstances()
	if err != nil {
		return nil, err
	}
	for _, inst := range l2 {
		// An unusable IPsec side is worth showing even while xl2tpd itself runs.
		running, reason := l2tp.GetManager().Status()
		out = append(out, VPNStatus{Id: inst.Id, Running: running && reason == "", Reason: reason})
	}
	ssh, err := s.DesiredSSHInstances()
	if err != nil {
		return nil, err
	}
	for _, inst := range ssh {
		running, reason := sshtunnel.GetManager().Status(inst.Id)
		out = append(out, VPNStatus{Id: inst.Id, Running: running, Reason: reasonIfDown(running, reason)})
	}
	st, err := s.DesiredSSTPInstances()
	if err != nil {
		return nil, err
	}
	for _, inst := range st {
		running, reason := sstp.GetManager().Status(inst.Id)
		out = append(out, VPNStatus{Id: inst.Id, Running: running, Reason: reasonIfDown(running, reason)})
	}
	oc, err := s.DesiredOCServInstances()
	if err != nil {
		return nil, err
	}
	for _, inst := range oc {
		running, reason := ocserv.GetManager().Status(inst.Id)
		out = append(out, VPNStatus{Id: inst.Id, Running: running, Reason: reasonIfDown(running, reason)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func reasonIfDown(running bool, reason string) string {
	if running {
		return ""
	}
	return reason
}
