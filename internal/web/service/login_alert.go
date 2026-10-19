package service

import (
	"net/netip"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// LoginAlertService remembers where each admin signs in from and raises
// login.new_location when it is somewhere new. The first sign-in only teaches.
type LoginAlertService struct {
	security  SecurityConfigService
	countryOf func(netip.Addr) string
}

func geoipCountry(addr netip.Addr) string {
	code, err := assetStore().CountryOf("geoip.dat", addr)
	if err != nil {
		logger.Debug("login alert: country lookup:", err)
	}
	return code
}

// remember stores the value and reports whether it is new, and whether the
// admin had any value of that kind before.
func remember(tx *gorm.DB, userId int, kind, value string, now int64) (isNew, hadAny bool, err error) {
	var before int64
	if err = tx.Model(&model.LoginSeen{}).Where("user_id = ? AND kind = ?", userId, kind).Count(&before).Error; err != nil {
		return false, false, err
	}
	row := &model.LoginSeen{UserId: userId, Kind: kind, Value: value, FirstSeen: now, LastSeen: now}
	res := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "value"}},
		DoUpdates: clause.Assignments(map[string]any{"last_seen": now}),
	}).Create(row)
	if res.Error != nil {
		return false, false, res.Error
	}
	var total int64
	err = tx.Model(&model.LoginSeen{}).Where("user_id = ? AND kind = ?", userId, kind).Count(&total).Error
	return total > before, before > 0, err
}

// Observe records a successful sign-in from ip and alerts when it is new.
func (s *LoginAlertService) Observe(user *model.User, ip string) {
	cfg, err := s.security.Get()
	if err != nil || !cfg.NewLocationAlert {
		return
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return
	}
	lookup := s.countryOf
	if lookup == nil {
		lookup = geoipCountry
	}
	country := lookup(addr)
	now := time.Now().UnixMilli()
	var newIP, newCountry bool
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		isNew, hadIP, err := remember(tx, user.Id, model.LoginSeenIP, addr.String(), now)
		if err != nil {
			return err
		}
		newIP = isNew && hadIP
		if country == "" {
			return nil
		}
		isNew, hadCountry, err := remember(tx, user.Id, model.LoginSeenCountry, country, now)
		newCountry = isNew && hadCountry
		return err
	})
	if err != nil {
		logger.Warning("login alert:", err)
		return
	}
	if (!newIP && !newCountry) || eventBus == nil {
		return
	}
	eventBus.Publish(eventbus.Event{
		Type:      eventbus.EventLoginNewLocation,
		Source:    user.Username,
		Data:      &eventbus.NewLoginLocationData{Username: user.Username, IP: addr.String(), Country: country, NewCountry: newCountry},
		Timestamp: time.Now(),
	})
}
