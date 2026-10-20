package sub

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// VPNAccount is a login on a VPN inbound served outside Xray (OpenVPN, IKEv2):
// those have no share link, the user imports a profile and logs in.
type VPNAccount struct {
	InboundId int    `json:"inboundId"`
	Protocol  string `json:"protocol"`
	Remark    string `json:"remark"`
	Server    string `json:"server"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	PSK       string `json:"psk,omitempty"`
	// Port and Fingerprint are what an SSH tunnel app asks for besides the login.
	Port        int    `json:"port,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

var errNoVPNProfile = errors.New("no vpn profile for this subscription")

func servesVPNAccounts(p model.Protocol) bool {
	return p == model.OpenVPN || p == model.IKEv2 || p == model.L2TP || p == model.SSH || p == model.SSTP || p == model.OCServ
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *SubService) addVPNAccounts(inbound *model.Inbound, clients []model.Client) {
	remark := inbound.Remark
	if remark == "" {
		remark = inbound.Tag
	}
	server := s.resolveInboundAddress(inbound)
	if inst, _, ok := ipsec.ServerOf(inbound); ok {
		server = inst.Address
	}
	// SSTP and AnyConnect clients check the certificate against the name they dialled.
	if inst, _, ok := sstp.ServerOf(inbound); ok {
		server = inst.Address
	}
	if inst, _, ok := ocserv.ServerOf(inbound); ok {
		server = inst.Address
	}
	var psk string
	if inst, _, ok := l2tp.ServerOf(inbound); ok {
		psk = inst.PSK
	}
	var port int
	var fingerprint string
	if inst, _, ok := sshtunnel.ServerOf(inbound); ok {
		port, fingerprint = inst.Port, sshtunnel.Fingerprint(inst.HostKey)
	}
	if inst, _, ok := sstp.ServerOf(inbound); ok {
		port = inst.Port
	}
	if inst, _, ok := ocserv.ServerOf(inbound); ok {
		port = inst.Port
	}
	targets := s.vpnTargets(inbound, server)
	for _, target := range targets {
		shownPort, shownRemark := port, remark
		if target.Port != 0 && port != 0 {
			shownPort = target.Port
		}
		if len(targets) > 1 && target.Remark != "" {
			shownRemark = remark + " · " + target.Remark
		}
		for _, c := range clients {
			s.vpnAccounts = append(s.vpnAccounts, VPNAccount{
				InboundId: inbound.Id, Protocol: string(inbound.Protocol), Remark: shownRemark,
				Server: target.Server, Username: c.Email, Password: c.Password, PSK: psk,
				Port: shownPort, Fingerprint: fingerprint,
			})
		}
	}
}

// vpnTarget is an address a client of a VPN inbound dials; Port 0 keeps the inbound's own.
type vpnTarget struct {
	Server string
	Port   int
	Remark string
}

// vpnTargets lists where the inbound's hosts say clients connect, or just own
// when it has none. A host without an address of its own keeps own, which for
// IKEv2, SSTP and AnyConnect is the name their certificate was issued for.
func (s *SubService) vpnTargets(inbound *model.Inbound, own string) []vpnTarget {
	eps := s.hostEndpoints(inbound, "raw")
	if len(eps) == 0 {
		return []vpnTarget{{Server: own}}
	}
	fallback := s.resolveInboundAddress(inbound)
	targets := make([]vpnTarget, 0, len(eps))
	seen := map[vpnTarget]bool{}
	for _, ep := range eps {
		t := vpnTarget{Server: own}
		if dest, _ := ep["dest"].(string); dest != "" && dest != fallback {
			t.Server = dest
		}
		if port, ok := ep["port"].(float64); ok && int(port) != inbound.Port {
			t.Port = int(port)
		}
		t.Remark, _ = ep["remark"].(string)
		key := vpnTarget{Server: t.Server, Port: t.Port}
		if !seen[key] {
			seen[key] = true
			targets = append(targets, t)
		}
	}
	return targets
}

func (s *SubService) vpnUsernames() []string {
	names := make([]string, 0, len(s.vpnAccounts))
	for _, p := range s.vpnAccounts {
		names = append(names, p.Username)
	}
	return names
}

// OpenVPNProfileFile builds the .ovpn for one of subId's OpenVPN inbounds.
func (s *SubService) OpenVPNProfileFile(subId string, inboundId int) (name, body string, err error) {
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil || ib.Protocol != model.OpenVPN {
		return "", "", errNoVPNProfile
	}
	if len(s.matchingClients(&ib, subId)) == 0 {
		return "", "", errNoVPNProfile
	}
	inst, _, ok := openvpn.ServerOf(&ib)
	if !ok {
		return "", "", errNoVPNProfile
	}
	remark := ib.Remark
	if remark == "" {
		remark = ib.Tag
	}
	name = strings.Trim(unsafeFileChars.ReplaceAllString(remark, "-"), "-")
	if name == "" {
		name = "openvpn-" + strconv.Itoa(ib.Id)
	}
	target := s.vpnTargets(&ib, s.resolveInboundAddress(&ib))[0]
	if target.Port != 0 {
		inst.Port = target.Port
	}
	return name + ".ovpn", openvpn.ClientProfile(inst, target.Server, remark), nil
}

// SSTPCAFile is the CA certificate Windows must trust for subId's SSTP
// inbound; only a subscription with an account on it may fetch it.
func (s *SubService) SSTPCAFile(subId string, inboundId int) (name, mime string, body []byte, err error) {
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil || ib.Protocol != model.SSTP {
		return "", "", nil, errNoVPNProfile
	}
	if len(s.matchingClients(&ib, subId)) == 0 {
		return "", "", nil, errNoVPNProfile
	}
	return service.SSTPCA(&ib)
}

// OCServCAFile is the CA certificate for subId's OpenConnect inbound; only a
// subscription with an account on it may fetch it.
func (s *SubService) OCServCAFile(subId string, inboundId int) (name, mime string, body []byte, err error) {
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil || ib.Protocol != model.OCServ {
		return "", "", nil, errNoVPNProfile
	}
	if len(s.matchingClients(&ib, subId)) == 0 {
		return "", "", nil, errNoVPNProfile
	}
	return service.OCServCA(&ib)
}

// IKEv2ProfileFile builds one format of the IKEv2 profile for subId's login
// on one of its IKEv2 inbounds.
func (s *SubService) IKEv2ProfileFile(subId string, inboundId int, format string) (name, mime string, body []byte, err error) {
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil || ib.Protocol != model.IKEv2 {
		return "", "", nil, errNoVPNProfile
	}
	clients := s.matchingClients(&ib, subId)
	if len(clients) == 0 {
		return "", "", nil, errNoVPNProfile
	}
	own := s.resolveInboundAddress(&ib)
	if inst, _, ok := ipsec.ServerOf(&ib); ok {
		own = inst.Address
	}
	return service.IKEv2Profile(&ib, clients[0].Email, format, s.vpnTargets(&ib, own)[0].Server)
}
