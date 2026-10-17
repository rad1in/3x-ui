package sub

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
)

// OpenVPNProfile is an OpenVPN inbound a subscription can download a profile
// for; OpenVPN has no share link, the user imports the file and logs in.
type OpenVPNProfile struct {
	InboundId int    `json:"inboundId"`
	Remark    string `json:"remark"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

var errNoOpenVPNProfile = errors.New("no openvpn profile for this subscription")

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *SubService) addOpenVPNProfiles(inbound *model.Inbound, clients []model.Client) {
	remark := inbound.Remark
	if remark == "" {
		remark = inbound.Tag
	}
	for _, c := range clients {
		s.openvpnProfiles = append(s.openvpnProfiles, OpenVPNProfile{
			InboundId: inbound.Id, Remark: remark, Username: c.Email, Password: c.Password,
		})
	}
}

func (s *SubService) openvpnUsernames() []string {
	names := make([]string, 0, len(s.openvpnProfiles))
	for _, p := range s.openvpnProfiles {
		names = append(names, p.Username)
	}
	return names
}

// OpenVPNProfileFile builds the .ovpn for one of subId's OpenVPN inbounds.
func (s *SubService) OpenVPNProfileFile(subId string, inboundId int) (name, body string, err error) {
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil || ib.Protocol != model.OpenVPN {
		return "", "", errNoOpenVPNProfile
	}
	if len(s.matchingClients(&ib, subId)) == 0 {
		return "", "", errNoOpenVPNProfile
	}
	inst, _, ok := openvpn.ServerOf(&ib)
	if !ok {
		return "", "", errNoOpenVPNProfile
	}
	remark := ib.Remark
	if remark == "" {
		remark = ib.Tag
	}
	name = strings.Trim(unsafeFileChars.ReplaceAllString(remark, "-"), "-")
	if name == "" {
		name = "openvpn-" + strconv.Itoa(ib.Id)
	}
	return name + ".ovpn", openvpn.ClientProfile(inst, s.resolveInboundAddress(&ib), remark), nil
}
