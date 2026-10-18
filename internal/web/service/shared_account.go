package service

import (
	"encoding/json"
	"net/netip"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

// SharedAccountService flags clients seen from many distinct addresses or countries
// inside a window. The live IP list only spans 30 minutes, so each scan folds it into
// client_ip_sightings, which keeps an address for the whole window.
type SharedAccountService struct {
	settings  SettingService
	countryOf func(netip.Addr) string
}

type sharedAccountConfig struct {
	windowHours  int
	minIPs       int
	minCountries int
}

func (c sharedAccountConfig) enabled() bool { return c.minIPs > 0 || c.minCountries > 0 }

func (s *SharedAccountService) config() sharedAccountConfig {
	cfg := sharedAccountConfig{}
	cfg.windowHours, _ = s.settings.getInt("sharedAccountWindowHours")
	cfg.minIPs, _ = s.settings.getInt("sharedAccountMinIPs")
	cfg.minCountries, _ = s.settings.getInt("sharedAccountMinCountries")
	cfg.windowHours = max(cfg.windowHours, 1)
	return cfg
}

// List returns the clients currently flagged, most addresses first.
func (s *SharedAccountService) List() ([]model.SharedAccountFlag, error) {
	flags := []model.SharedAccountFlag{}
	err := database.GetDB().Order("ip_count DESC, email").Find(&flags).Error
	return flags, err
}

// Scan folds the live IP list into the sightings, drops what left the window and
// reconciles the flags, alerting for every client that newly crossed a threshold.
func (s *SharedAccountService) Scan(now time.Time) error {
	cfg := s.config()
	db := database.GetDB()
	if !cfg.enabled() {
		if err := db.Where("1 = 1").Delete(&model.SharedAccountFlag{}).Error; err != nil {
			return err
		}
		return db.Where("1 = 1").Delete(&model.ClientIPSighting{}).Error
	}
	cutoff := now.Add(-time.Duration(cfg.windowHours) * time.Hour).Unix()
	var newlyFlagged []model.SharedAccountFlag
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := s.recordSightings(tx, cfg, cutoff); err != nil {
			return err
		}
		if err := tx.Where("last_seen < ?", cutoff).Delete(&model.ClientIPSighting{}).Error; err != nil {
			return err
		}
		var err error
		newlyFlagged, err = s.reconcileFlags(tx, cfg, now)
		return err
	})
	if err != nil {
		return err
	}
	for _, f := range newlyFlagged {
		s.publish(f, cfg.windowHours)
	}
	return nil
}

func (s *SharedAccountService) lookupCountry(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	lookup := s.countryOf
	if lookup == nil {
		lookup = geoipCountry
	}
	return lookup(addr)
}

func (s *SharedAccountService) recordSightings(tx *gorm.DB, cfg sharedAccountConfig, cutoff int64) error {
	var rows []model.InboundClientIps
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	var known []model.ClientIPSighting
	if err := tx.Find(&known).Error; err != nil {
		return err
	}
	seenCountry := make(map[string]string, len(known))
	for _, k := range known {
		seenCountry[k.Email+"\x00"+k.IP] = k.Country
	}
	lookedUp := map[string]string{}
	var batch []model.ClientIPSighting
	for _, row := range rows {
		var entries []clientIpEntry
		if json.Unmarshal([]byte(row.Ips), &entries) != nil {
			continue
		}
		for _, e := range entries {
			if e.IP == "" || e.Timestamp < cutoff {
				continue
			}
			country, ok := seenCountry[row.ClientEmail+"\x00"+e.IP]
			if !ok && cfg.minCountries > 0 {
				if country, ok = lookedUp[e.IP]; !ok {
					country = s.lookupCountry(e.IP)
					lookedUp[e.IP] = country
				}
			}
			batch = append(batch, model.ClientIPSighting{Email: row.ClientEmail, IP: e.IP, Country: country, LastSeen: e.Timestamp})
		}
	}
	if len(batch) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}, {Name: "ip"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen"}),
	}).CreateInBatches(batch, 200).Error
}

func (s *SharedAccountService) reconcileFlags(tx *gorm.DB, cfg sharedAccountConfig, now time.Time) ([]model.SharedAccountFlag, error) {
	var sightings []model.ClientIPSighting
	if err := tx.Find(&sightings).Error; err != nil {
		return nil, err
	}
	type tally struct {
		ips       int
		countries map[string]struct{}
		lastSeen  int64
	}
	byEmail := map[string]*tally{}
	for _, sg := range sightings {
		t := byEmail[sg.Email]
		if t == nil {
			t = &tally{countries: map[string]struct{}{}}
			byEmail[sg.Email] = t
		}
		t.ips++
		t.lastSeen = max(t.lastSeen, sg.LastSeen)
		if sg.Country != "" {
			t.countries[sg.Country] = struct{}{}
		}
	}
	var existing []model.SharedAccountFlag
	if err := tx.Find(&existing).Error; err != nil {
		return nil, err
	}
	firstFlagged := make(map[string]int64, len(existing))
	for _, f := range existing {
		firstFlagged[f.Email] = f.FirstFlagged
	}

	var current []model.SharedAccountFlag
	var created []model.SharedAccountFlag
	for email, t := range byEmail {
		if !(cfg.minIPs > 0 && t.ips >= cfg.minIPs) && !(cfg.minCountries > 0 && len(t.countries) >= cfg.minCountries) {
			continue
		}
		codes := make([]string, 0, len(t.countries))
		for c := range t.countries {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		flag := model.SharedAccountFlag{
			Email: email, IPCount: t.ips, CountryCount: len(codes), Countries: strings.Join(codes, ","),
			FirstFlagged: now.UnixMilli(), LastSeen: t.lastSeen * 1000,
		}
		if first, ok := firstFlagged[email]; ok {
			flag.FirstFlagged = first
		} else {
			created = append(created, flag)
		}
		current = append(current, flag)
	}
	if err := tx.Where("1 = 1").Delete(&model.SharedAccountFlag{}).Error; err != nil {
		return nil, err
	}
	if len(current) > 0 {
		if err := tx.CreateInBatches(current, 200).Error; err != nil {
			return nil, err
		}
	}
	sort.Slice(created, func(i, j int) bool { return created[i].Email < created[j].Email })
	return created, nil
}

func (s *SharedAccountService) publish(f model.SharedAccountFlag, windowHours int) {
	if eventBus == nil {
		return
	}
	eventBus.Publish(eventbus.Event{
		Type:   eventbus.EventClientSharedAccount,
		Source: f.Email,
		Data: &eventbus.SharedAccountData{
			Email: f.Email, IPCount: f.IPCount, CountryCount: f.CountryCount, Countries: f.Countries, WindowHours: windowHours,
		},
		Timestamp: time.Now(),
	})
}
