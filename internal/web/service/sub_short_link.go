package service

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
)

// SubShortLinkPath is where the subscription server answers short links.
const SubShortLinkPath = "/s/"

// 10 base62 chars is ~59 bits: short, yet not enumerable through the
// subscription server, which also rate-limits.
const subShortLinkTokenLen = 10

var (
	ErrSubShortLinkNotFound  = errors.New("subscription short link not found")
	ErrSubShortLinksDisabled = errors.New("short subscription links are turned off in the subscription settings")
)

// SubShortLinkView is a subscription's short link as the panel shows it.
type SubShortLinkView struct {
	Token string `json:"token" example:"Ab3dE5gH9k"`
	URL   string `json:"url" example:"https://sub.example.com:2096/s/Ab3dE5gH9k"`
}

type SubShortLinkService struct {
	settingService SettingService
}

func (s *SettingService) GetSubShortLinkEnable() (bool, error) {
	return s.getBool("subShortLinkEnable")
}

func (s *SubShortLinkService) Ensure(subID string) (model.SubShortLink, error) {
	subID = strings.TrimSpace(subID)
	if subID == "" {
		return model.SubShortLink{}, ErrSubShortLinkNotFound
	}
	db := database.GetDB()
	link, err := s.Get(subID)
	if err == nil {
		return link, nil
	}
	if !errors.Is(err, ErrSubShortLinkNotFound) {
		return model.SubShortLink{}, err
	}
	var clients int64
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", subID).Count(&clients).Error; err != nil {
		return model.SubShortLink{}, err
	}
	if clients == 0 {
		return model.SubShortLink{}, ErrSubShortLinkNotFound
	}
	link = model.SubShortLink{SubID: subID, Token: random.Seq(subShortLinkTokenLen), CreatedAt: time.Now().UnixMilli()}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
		return model.SubShortLink{}, err
	}
	// A concurrent Ensure may have won the insert; return the stored row either way.
	err = db.Where("sub_id = ?", subID).First(&link).Error
	return link, err
}

func (s *SubShortLinkService) Get(subID string) (model.SubShortLink, error) {
	var link model.SubShortLink
	err := database.GetDB().Where("sub_id = ?", subID).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return link, ErrSubShortLinkNotFound
	}
	return link, err
}

func (s *SubShortLinkService) Revoke(subID string) error {
	return database.GetDB().Where("sub_id = ?", subID).Delete(&model.SubShortLink{}).Error
}

func (s *SubShortLinkService) Resolve(token string) (string, error) {
	if token == "" {
		return "", ErrSubShortLinkNotFound
	}
	var link model.SubShortLink
	err := database.GetDB().Where("token = ?", token).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrSubShortLinkNotFound
	}
	return link.SubID, err
}

// View pairs the token with its public URL: a configured subURI's origin (the
// reverse proxy), else the subscription server's own origin for host.
func (s *SubShortLinkService) View(link model.SubShortLink, host string) SubShortLinkView {
	base := s.settingService.BuildSubURIBase(host)
	if subURI, _ := s.settingService.GetSubURI(); subURI != "" {
		if origin := uriOrigin(subURI); origin != "" {
			base = origin
		}
	}
	return SubShortLinkView{Token: link.Token, URL: strings.TrimRight(base, "/") + SubShortLinkPath + link.Token}
}

func uriOrigin(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
