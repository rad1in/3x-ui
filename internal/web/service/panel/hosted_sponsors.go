package panel

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// hostedSponsors is the owner's live sponsor set as a hosted panel reads it:
// feed-form logo names plus the logo bytes, so the panel never fetches them.
type hostedSponsors struct {
	List  SponsorList       `json:"list"`
	Logos map[string][]byte `json:"logos"`
}

// PublishHostedSponsors copies the owner's sponsors into every hosted panel.
func PublishHostedSponsors() {
	if hosted.Enabled() {
		return
	}
	var ids []int
	if err := database.GetDB().Model(&model.HostedPanel{}).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return
	}
	data, err := hostedSponsorSnapshot()
	if err != nil {
		logger.Debug("hosted sponsors: snapshot:", err)
		return
	}
	for _, id := range ids {
		if err := hosted.WriteSponsors(hostedPanelDir(id), data); err != nil {
			logger.Debug("hosted sponsors: write panel", id, err)
		}
	}
}

func hostedSponsorSnapshot() ([]byte, error) {
	s := &PanelService{}
	live, err := s.GetSponsors()
	if err != nil {
		return nil, err
	}
	snap := hostedSponsors{
		List:  SponsorList{Contact: live.Contact, Sponsors: make([]Sponsor, 0, len(live.Sponsors))},
		Logos: map[string][]byte{},
	}
	for _, sp := range live.Sponsors {
		name := strings.TrimPrefix(sp.Logo, sponsorLogoPath)
		sp.Logo = ""
		if name != "" {
			if data, _, err := s.GetSponsorLogo(name); err == nil {
				snap.Logos[name], sp.Logo = data, name
			}
		}
		snap.List.Sponsors = append(snap.List.Sponsors, sp)
	}
	return json.Marshal(snap)
}

func readHostedSponsors(dir string) hostedSponsors {
	var snap hostedSponsors
	data, err := hosted.ReadSponsors(dir)
	if err == nil {
		err = json.Unmarshal(data, &snap)
	}
	if err != nil {
		logger.Debug("hosted sponsors: read:", err)
	}
	return snap
}

func hostedActiveSponsors(dir string) *SponsorList {
	snap := readHostedSponsors(dir)
	return activeSponsors(&snap.List, sponsorNow())
}

func hostedSponsorLogo(dir, name string) ([]byte, string, error) {
	snap := readHostedSponsors(dir)
	live := activeSponsors(&snap.List, sponsorNow())
	data, ok := snap.Logos[name]
	if !ok || !slices.ContainsFunc(live.Sponsors, func(sp Sponsor) bool { return sp.Logo == sponsorLogoPath+name }) {
		return nil, "", ErrSponsorLogoUnknown
	}
	return sniffSponsorLogo(name, data)
}
