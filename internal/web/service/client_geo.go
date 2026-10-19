package service

import (
	"encoding/json"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// clientGeoWindow is how recently an address must have been seen to count as connected.
const clientGeoWindow = 2 * time.Minute

// CountryCount is how many clients and addresses are connected from one country.
type CountryCount struct {
	Country   string `json:"country" example:"DE"`
	Clients   int    `json:"clients" example:"4"`
	Addresses int    `json:"addresses" example:"6"`
}

// ClientGeo is the connected clients grouped by country. Available is false
// when no geoip database could be read, so the card can say so.
type ClientGeo struct {
	Available bool           `json:"available" example:"true"`
	Countries []CountryCount `json:"countries"`
	Unlocated int            `json:"unlocated" example:"1"`
}

// ClientGeoService places the addresses clients are connected from on a country.
// A lookup scans the whole database, so results are kept for as long as the
// address stays connected.
type ClientGeoService struct {
	countryOf func(netip.Addr) (string, error)

	mu    sync.Mutex
	cache map[string]string
}

func (s *ClientGeoService) lookup(addr netip.Addr) (string, error) {
	if s.countryOf != nil {
		return s.countryOf(addr)
	}
	return assetStore().CountryOf("geoip.dat", addr)
}

// Snapshot groups the addresses seen within the last two minutes by country.
func (s *ClientGeoService) Snapshot(now time.Time) (*ClientGeo, error) {
	var rows []model.InboundClientIps
	if err := database.GetDB().Find(&rows).Error; err != nil {
		return nil, err
	}
	cutoff := now.Add(-clientGeoWindow).Unix()
	emailsByIP := map[string]map[string]struct{}{}
	for _, row := range rows {
		var entries []clientIpEntry
		if json.Unmarshal([]byte(row.Ips), &entries) != nil {
			continue
		}
		for _, e := range entries {
			if e.IP == "" || e.Timestamp < cutoff {
				continue
			}
			if emailsByIP[e.IP] == nil {
				emailsByIP[e.IP] = map[string]struct{}{}
			}
			emailsByIP[e.IP][row.ClientEmail] = struct{}{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]string, len(emailsByIP))
	geo := &ClientGeo{Available: true, Countries: []CountryCount{}}
	clients := map[string]map[string]struct{}{}
	addresses := map[string]int{}
	for ip, emails := range emailsByIP {
		country, cached := s.cache[ip]
		if !cached {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				continue
			}
			if country, err = s.lookup(addr); err != nil {
				s.cache = nil
				return &ClientGeo{Available: false, Countries: []CountryCount{}}, nil
			}
		}
		next[ip] = country
		if country == "" {
			geo.Unlocated++
			continue
		}
		addresses[country]++
		if clients[country] == nil {
			clients[country] = map[string]struct{}{}
		}
		for email := range emails {
			clients[country][email] = struct{}{}
		}
	}
	s.cache = next
	for country, n := range addresses {
		geo.Countries = append(geo.Countries, CountryCount{Country: country, Clients: len(clients[country]), Addresses: n})
	}
	sort.Slice(geo.Countries, func(i, j int) bool {
		a, b := geo.Countries[i], geo.Countries[j]
		if a.Clients != b.Clients {
			return a.Clients > b.Clients
		}
		return a.Country < b.Country
	})
	return geo, nil
}
