package panel

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// anyLanguage keys a locally written title/text: the owner writes it once and
// every UI language shows it (pickLocale falls back to it).
const anyLanguage = "*"

const (
	localSponsorPrefix    = "local-"
	sponsorLogoMaxBytes   = 64 << 10
	sponsorNameMaxRunes   = 64
	sponsorTitleMaxRunes  = 120
	sponsorTextMaxRunes   = 300
	sponsorContactMaxLen  = 512
	sponsorFeedURLMaxLen  = 1024
	sponsorNoEndYear      = 9999
	sponsorLogoDataPrefix = "data:image/"
)

var sponsorLogoExt = map[string]string{"png": "png", "jpeg": "jpg", "webp": "webp"}

// SponsorConfig is the owner-level sponsor setup: where sponsors come from and
// the "become a sponsor" contact link.
type SponsorConfig struct {
	FeedURL string `json:"feedUrl" example:""`
	Contact string `json:"contact" example:"https://t.me/example"`
}

// SponsorAdminView is what the sponsor manager page loads.
type SponsorAdminView struct {
	Records []model.SponsorRecord `json:"records"`
	Config  SponsorConfig         `json:"config"`
}

func (s *PanelService) SponsorAdmin() (*SponsorAdminView, error) {
	view := &SponsorAdminView{Records: []model.SponsorRecord{}}
	if err := database.GetDB().Order("id ASC").Find(&view.Records).Error; err != nil {
		return nil, err
	}
	settings := &service.SettingService{}
	var err error
	if view.Config.FeedURL, err = settings.GetSponsorFeedURL(); err != nil {
		return nil, err
	}
	if view.Config.Contact, err = settings.GetSponsorContact(); err != nil {
		return nil, err
	}
	return view, nil
}

// SaveSponsorRecord creates the record when Id is 0 and replaces it otherwise.
func (s *PanelService) SaveSponsorRecord(r model.SponsorRecord) (model.SponsorRecord, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Link = strings.TrimSpace(r.Link)
	if err := validateSponsorRecord(r); err != nil {
		return model.SponsorRecord{}, err
	}
	db := database.GetDB()
	if r.Id == 0 {
		return r, db.Create(&r).Error
	}
	res := db.Model(&model.SponsorRecord{}).Where("id = ?", r.Id).Select("*").Updates(&r)
	if res.Error != nil {
		return model.SponsorRecord{}, res.Error
	}
	if res.RowsAffected == 0 {
		return model.SponsorRecord{}, errors.New("sponsor not found")
	}
	return r, nil
}

func (s *PanelService) DeleteSponsorRecord(id int) error {
	return database.GetDB().Delete(&model.SponsorRecord{}, id).Error
}

func (s *PanelService) SetSponsorConfig(c SponsorConfig) error {
	c.FeedURL = strings.TrimSpace(c.FeedURL)
	c.Contact = strings.TrimSpace(c.Contact)
	if err := validateSponsorFeedURL(c.FeedURL); err != nil {
		return err
	}
	if c.Contact != "" && (!strings.HasPrefix(c.Contact, "https://") || len(c.Contact) > sponsorContactMaxLen) {
		return errors.New("contact link must be an https:// URL")
	}
	return (&service.SettingService{}).SetSponsorSettings(c.FeedURL, c.Contact)
}

func validateSponsorRecord(r model.SponsorRecord) error {
	switch {
	case r.Name == "" || utf8.RuneCountInString(r.Name) > sponsorNameMaxRunes:
		return errors.New("sponsor name must be 1 to 64 characters")
	case utf8.RuneCountInString(r.Title) > sponsorTitleMaxRunes:
		return errors.New("sponsor title must be at most 120 characters")
	case utf8.RuneCountInString(r.Text) > sponsorTextMaxRunes:
		return errors.New("sponsor text must be at most 300 characters")
	case !isHTTPSURL(r.Link):
		return errors.New("sponsor link must be an https:// URL")
	case len(r.Slots) == 0:
		return errors.New("pick at least one place to show the sponsor")
	case r.FromMs < 0 || r.UntilMs < 0:
		return errors.New("sponsor dates must not be negative")
	case r.FromMs > 0 && r.UntilMs > 0 && r.UntilMs <= r.FromMs:
		return errors.New("sponsor end must be after its start")
	}
	for _, slot := range r.Slots {
		if !sponsorSlots[slot] {
			return errors.New("unknown sponsor slot: " + slot)
		}
	}
	if r.Logo != "" {
		return entity.ValidateImageDataURL(r.Logo, sponsorLogoMaxBytes)
	}
	return nil
}

func isHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// Plain http is accepted only for loopback, where a child panel reads its master on the same box.
func validateSponsorFeedURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > sponsorFeedURLMaxLen || u.Hostname() == "" {
		return errors.New("sponsor feed must be a full URL")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil
		}
	}
	return errors.New("sponsor feed must use https://, or http:// on this server only")
}

func (s *PanelService) localSponsors() (*SponsorList, error) {
	raw, err := localSponsorList()
	if err != nil {
		return nil, err
	}
	return activeSponsors(raw, sponsorNow()), nil
}

// SponsorFeed is the live local sponsor set in feed form: bare logo file names
// that resolve against the feed's logos/ directory.
func (s *PanelService) SponsorFeed() (*SponsorList, error) {
	live, err := s.localSponsors()
	if err != nil {
		return nil, err
	}
	for i := range live.Sponsors {
		live.Sponsors[i].Logo = strings.TrimPrefix(live.Sponsors[i].Logo, sponsorLogoPath)
	}
	return live, nil
}

// GetSponsorFeedLogo serves the logo of a live local sponsor.
func (s *PanelService) GetSponsorFeedLogo(name string) ([]byte, string, error) {
	if !sponsorLogoRe.MatchString(name) {
		return nil, "", ErrSponsorLogoUnknown
	}
	live, err := s.localSponsors()
	if err != nil {
		return nil, "", err
	}
	if !slices.ContainsFunc(live.Sponsors, func(sp Sponsor) bool { return sp.Logo == sponsorLogoPath+name }) {
		return nil, "", ErrSponsorLogoUnknown
	}
	idText, _, _ := strings.Cut(strings.TrimPrefix(name, localSponsorPrefix), ".")
	id, err := strconv.Atoi(idText)
	if err != nil {
		return nil, "", ErrSponsorLogoUnknown
	}
	var rec model.SponsorRecord
	if err := database.GetDB().Select("logo").First(&rec, id).Error; err != nil {
		return nil, "", ErrSponsorLogoUnknown
	}
	_, payload, ok := strings.Cut(rec.Logo, ";base64,")
	if !ok {
		return nil, "", ErrSponsorLogoUnknown
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", err
	}
	return sniffSponsorLogo(name, data)
}

func localSponsorList() (*SponsorList, error) {
	var records []model.SponsorRecord
	if err := database.GetDB().Order("id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	contact, err := (&service.SettingService{}).GetSponsorContact()
	if err != nil {
		return nil, err
	}
	list := &SponsorList{Contact: contact, Sponsors: make([]Sponsor, 0, len(records))}
	for _, r := range records {
		list.Sponsors = append(list.Sponsors, sponsorFromRecord(r))
	}
	return list, nil
}

func sponsorFromRecord(r model.SponsorRecord) Sponsor {
	enable := r.Enable
	sp := Sponsor{
		ID:     localSponsorPrefix + strconv.Itoa(r.Id),
		Name:   r.Name,
		Enable: &enable,
		Slots:  r.Slots,
		Until:  time.Date(sponsorNoEndYear, 1, 1, 0, 0, 0, 0, time.UTC),
		Title:  map[string]string{},
		Text:   map[string]string{},
		Link:   r.Link,
	}
	if r.FromMs > 0 {
		from := time.UnixMilli(r.FromMs).UTC()
		sp.From = &from
	}
	if r.UntilMs > 0 {
		sp.Until = time.UnixMilli(r.UntilMs).UTC()
	}
	if r.Title != "" {
		sp.Title[anyLanguage] = r.Title
	}
	if r.Text != "" {
		sp.Text[anyLanguage] = r.Text
	}
	if mime, _, ok := strings.Cut(strings.TrimPrefix(r.Logo, sponsorLogoDataPrefix), ";"); ok {
		if ext, known := sponsorLogoExt[mime]; known {
			sp.Logo = sp.ID + "." + ext
		}
	}
	return sp
}
