package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/subsign"
)

var ErrSubSigningOff = errors.New("signed subscription links are not enabled")

// SignedSubLink is a subscription URL carrying its signature. ExpiresAt is unix
// ms, 0 for a link that never expires.
type SignedSubLink struct {
	URL       string `json:"url" example:"https://sub.example.com:2096/sub/Ab3dE5gH9k?exp=1792411200&sig=Zk3Wq1"`
	ExpiresAt int64  `json:"expiresAt" example:"1792411200000"`
}

// SubSigningService issues signed subscription links.
type SubSigningService struct {
	security SecurityConfigService
	settings SettingService
}

// prefix is the URL up to and including the trailing slash before the id.
func (s *SubSigningService) prefix(host string) string {
	if uri, _ := s.settings.GetSubURI(); uri != "" {
		return strings.TrimRight(uri, "/") + "/"
	}
	path, _ := s.settings.GetSubPath()
	path = "/" + strings.Trim(path, "/") + "/"
	return s.settings.BuildSubURIBase(host) + path
}

// SignedURL signs the subscription id; ttl 0 issues a link that never expires.
func (s *SubSigningService) SignedURL(subID string, ttl time.Duration, host string) (*SignedSubLink, error) {
	cfg, err := s.security.Get()
	if err != nil {
		return nil, err
	}
	if !cfg.SubSigningEnable || cfg.SubSigningKey == "" {
		return nil, ErrSubSigningOff
	}
	subID = strings.TrimSpace(subID)
	if subID == "" {
		return nil, errors.New("subscription id is required")
	}
	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).Unix()
	}
	query := url.Values{"exp": {fmt.Sprint(exp)}, "sig": {subsign.Sign([]byte(cfg.SubSigningKey), subID, exp)}}
	link := &SignedSubLink{URL: s.prefix(host) + url.PathEscape(subID) + "?" + query.Encode()}
	if exp > 0 {
		link.ExpiresAt = exp * 1000
	}
	return link, nil
}
