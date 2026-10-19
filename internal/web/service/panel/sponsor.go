package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// Sponsor is one paid placement listed in the owner's sponsors.json.
type Sponsor struct {
	ID     string            `json:"id" example:"acme-2026-10"`
	Name   string            `json:"name" example:"Acme VPS"`
	Enable *bool             `json:"enable,omitempty" example:"true"`
	Slots  []string          `json:"slots"`
	From   *time.Time        `json:"from,omitempty" example:"2026-10-01T00:00:00Z"`
	Until  time.Time         `json:"until" example:"2026-11-01T00:00:00Z"`
	Logo   string            `json:"logo,omitempty" example:"/sponsors/logo/acme.png"`
	Title  map[string]string `json:"title"`
	Text   map[string]string `json:"text"`
	Link   string            `json:"link" example:"https://acme.example/?utm_source=eternal"`
}

// SponsorList is the active sponsor set plus the contact link for new sponsors.
type SponsorList struct {
	Contact  string    `json:"contact,omitempty" example:"https://t.me/example"`
	Sponsors []Sponsor `json:"sponsors"`
}

const (
	sponsorsTTL      = time.Hour
	sponsorsErrTTL   = 10 * time.Minute
	maxSponsorsBytes = 256 << 10
	maxLogoBytes     = 256 << 10
	sponsorLogoPath  = "/sponsors/logo/"
)

// DefaultSponsorFeedURL is the only sponsor source: the project owner's
// sponsors.json, with logos in the logos/ directory beside it. No panel can write it.
const DefaultSponsorFeedURL = "https://raw.githubusercontent.com/rad1in/sponsors/main/sponsors.json"

// errSponsorFeedMissing is a 404: a feed not published yet means no sponsors.
var errSponsorFeedMissing = errors.New("sponsor feed not found")

// ErrSponsorLogoUnknown rejects logo names not used by an active sponsor.
var ErrSponsorLogoUnknown = errors.New("unknown sponsor logo")

type sponsorLogo struct {
	data        []byte
	contentType string
	err         error
	retryAt     time.Time
}

var (
	sponsorNow  = time.Now
	sponsorFeed = DefaultSponsorFeedURL

	// The panel proxies feed logos: CSP stays 'self' and no third party sees admin IPs.
	sponsorLogoRe = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*\.(png|webp|jpg)$`)
	sponsorSlots  = map[string]bool{"dashboard": true, "page": true, "login": true}

	sponsorsMu      sync.Mutex
	sponsorsSource  string
	sponsorsRaw     *SponsorList
	sponsorsErr     error
	sponsorsRetryAt time.Time

	logosMu sync.Mutex
	logos   = map[string]sponsorLogo{}
)

// GetSponsors returns the currently active sponsors from the owner's feed. The
// window is re-checked on every call so a slot starts exactly at From and ends at Until.
func (s *PanelService) GetSponsors() (*SponsorList, error) {
	raw, err := cachedSponsors(sponsorFeed)
	if err != nil {
		return nil, err
	}
	return activeSponsors(raw, sponsorNow()), nil
}

// sponsorFeedLogoBase follows the feed layout: sponsors.json beside a logos/ directory.
func sponsorFeedLogoBase(feed string) string {
	return feed[:strings.LastIndex(feed, "/")+1] + "logos/"
}

func cachedSponsors(feed string) (*SponsorList, error) {
	sponsorsMu.Lock()
	defer sponsorsMu.Unlock()
	now := sponsorNow()
	if feed != sponsorsSource {
		sponsorsSource, sponsorsRaw, sponsorsErr, sponsorsRetryAt = feed, nil, nil, time.Time{}
	}
	if now.Before(sponsorsRetryAt) {
		return sponsorsRaw, sponsorsErr
	}
	list, err := fetchSponsors(feed)
	switch {
	case err == nil:
		sponsorsRaw, sponsorsErr, sponsorsRetryAt = list, nil, now.Add(sponsorsTTL)
	case sponsorsRaw != nil:
		// An upstream blip keeps the last good list, so paid slots do not blink out.
		logger.Debug("sponsors refresh failed, keeping last list:", err)
		sponsorsRetryAt = now.Add(sponsorsErrTTL)
	default:
		sponsorsErr, sponsorsRetryAt = err, now.Add(sponsorsErrTTL)
	}
	return sponsorsRaw, sponsorsErr
}

// GetSponsorLogo returns the image bytes for a logo of a currently active sponsor.
func (s *PanelService) GetSponsorLogo(name string) ([]byte, string, error) {
	// Validated here, not only via list membership, so name can never carry a path or URL.
	if !sponsorLogoRe.MatchString(name) {
		return nil, "", ErrSponsorLogoUnknown
	}
	sponsors, err := s.GetSponsors()
	if err != nil {
		return nil, "", err
	}
	if !slices.ContainsFunc(sponsors.Sponsors, func(sp Sponsor) bool { return sp.Logo == sponsorLogoPath+name }) {
		return nil, "", ErrSponsorLogoUnknown
	}
	logosMu.Lock()
	defer logosMu.Unlock()
	now := sponsorNow()
	key := sponsorFeed + "\x00" + name
	l, ok := logos[key]
	if ok && now.Before(l.retryAt) {
		return l.data, l.contentType, l.err
	}
	// Failures are cached too: this route is public and each miss is an outbound fetch.
	data, contentType, err := fetchSponsorLogo(sponsorFeedLogoBase(sponsorFeed), name)
	switch {
	case err == nil:
		l = sponsorLogo{data: data, contentType: contentType, retryAt: now.Add(sponsorsTTL)}
	case l.data != nil:
		l.retryAt = now.Add(sponsorsErrTTL)
	default:
		l = sponsorLogo{err: err, retryAt: now.Add(sponsorsErrTTL)}
	}
	logos[key] = l
	return l.data, l.contentType, l.err
}

func fetchSponsorLogo(base, name string) ([]byte, string, error) {
	data, err := fetchLimited(base+name, maxLogoBytes)
	if err != nil {
		return nil, "", err
	}
	return sniffSponsorLogo(name, data)
}

func sniffSponsorLogo(name string, data []byte) ([]byte, string, error) {
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/webp", "image/jpeg":
		return data, contentType, nil
	default:
		return nil, "", fmt.Errorf("sponsor logo %s has content type %s", name, contentType)
	}
}

func fetchLimited(url string, limit int) ([]byte, error) {
	client := (&service.SettingService{}).NewProxiedHTTPClient(10 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", errSponsorFeedMissing, url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s returned status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", url, limit)
	}
	return body, nil
}

func fetchSponsors(feed string) (*SponsorList, error) {
	body, err := fetchLimited(feed, maxSponsorsBytes)
	if errors.Is(err, errSponsorFeedMissing) {
		return &SponsorList{Sponsors: []Sponsor{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var list SponsorList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func activeSponsors(raw *SponsorList, now time.Time) *SponsorList {
	out := &SponsorList{Sponsors: []Sponsor{}}
	if raw == nil {
		return out
	}
	if strings.HasPrefix(raw.Contact, "https://") {
		out.Contact = raw.Contact
	}
	for _, sp := range raw.Sponsors {
		// A missing enable counts as on, so a forgotten field never hides a paid slot.
		disabled := sp.Enable != nil && !*sp.Enable
		notStarted := sp.From != nil && now.Before(*sp.From)
		if disabled || notStarted || sp.ID == "" || !now.Before(sp.Until) || !strings.HasPrefix(sp.Link, "https://") {
			continue
		}
		// A bad logo name drops only the logo; the paid slot still renders with its initial.
		if sponsorLogoRe.MatchString(sp.Logo) {
			sp.Logo = sponsorLogoPath + sp.Logo
		} else {
			sp.Logo = ""
		}
		slots := make([]string, 0, len(sp.Slots))
		for _, slot := range sp.Slots {
			if sponsorSlots[slot] {
				slots = append(slots, slot)
			}
		}
		if len(slots) == 0 {
			continue
		}
		sp.Slots = slots
		out.Sponsors = append(out.Sponsors, sp)
	}
	return out
}
