package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

var (
	ErrHostedInboundLimit = errors.New("this panel has reached its inbound limit")
	ErrHostedClientLimit  = errors.New("this panel has reached its client limit")
	ErrHostedPortRange    = errors.New("this port is outside the range allowed for this panel")
	ErrHostedBlocked      = errors.New("this panel is suspended by its provider")
	ErrHostedForbidden    = errors.New("this action is not available on a hosted panel")
)

// Kept outside AllSetting so the hosted panel's own admin can neither see nor reset them.
const (
	hostedUsedKey  = "hostedUsedBytes"
	hostedEpochKey = "hostedUsageEpoch"
)

// hostedHalted marks an Xray stop forced by the limits, so it restarts once they clear.
var hostedHalted atomic.Bool

// hostedLimits returns the limits when this panel is hosted; ok is false otherwise.
func hostedLimits() (limits hosted.Limits, ok bool, err error) {
	dir := hosted.Dir()
	if dir == "" {
		return hosted.Limits{}, false, nil
	}
	limits, err = hosted.ReadLimits(dir)
	if err != nil {
		return hosted.Limits{}, true, fmt.Errorf("%w (%s)", ErrHostedBlocked, hosted.BlockedNoLimits)
	}
	return limits, true, nil
}

func checkHostedInboundPort(ib *model.Inbound) error {
	limits, ok, err := hostedLimits()
	if !ok || err != nil || ib.NodeID != nil {
		return err
	}
	if !limits.PortAllowed(ib.Port) {
		return fmt.Errorf("%w (%d-%d)", ErrHostedPortRange, limits.PortMin, limits.PortMax)
	}
	return nil
}

func checkHostedInboundCreate(tx *gorm.DB, ib *model.Inbound) error {
	if err := checkHostedInboundPort(ib); err != nil {
		return err
	}
	limits, ok, _ := hostedLimits()
	if !ok || limits.MaxInbounds <= 0 || ib.NodeID != nil {
		return nil
	}
	var count int64
	if err := tx.Model(&model.Inbound{}).Where("node_id IS NULL").Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(limits.MaxInbounds) {
		return fmt.Errorf("%w (%d)", ErrHostedInboundLimit, limits.MaxInbounds)
	}
	return nil
}

func checkHostedNewClients(tx *gorm.DB, adding int) error {
	limits, ok, err := hostedLimits()
	if !ok || err != nil || adding <= 0 || limits.MaxClients <= 0 {
		return err
	}
	var count int64
	if err := tx.Model(&model.ClientRecord{}).Count(&count).Error; err != nil {
		return err
	}
	if count+int64(adding) > int64(limits.MaxClients) {
		return fmt.Errorf("%w (%d)", ErrHostedClientLimit, limits.MaxClients)
	}
	return nil
}

// HostedService meters a hosted panel against the limits its owner set.
type HostedService struct {
	settingService SettingService
}

// HostedView is the hosted panel's own view of its limits and usage.
type HostedView struct {
	Enabled   bool          `json:"enabled" example:"true"`
	Limits    hosted.Limits `json:"limits"`
	Inbounds  int64         `json:"inbounds" example:"2"`
	Clients   int64         `json:"clients" example:"40"`
	UsedBytes int64         `json:"usedBytes" example:"1073741824"`
	Blocked   string        `json:"blocked" example:""`
}

// usedBytes returns the metered total, zeroing it when the owner bumped the epoch.
func (h *HostedService) usedBytes(limits hosted.Limits) (int64, error) {
	epochText, err := h.settingService.getString(hostedEpochKey)
	if err != nil {
		return 0, err
	}
	if epochText != strconv.Itoa(limits.UsageEpoch) {
		if err := h.settingService.setString(hostedUsedKey, "0"); err != nil {
			return 0, err
		}
		return 0, h.settingService.setString(hostedEpochKey, strconv.Itoa(limits.UsageEpoch))
	}
	text, err := h.settingService.getString(hostedUsedKey)
	if err != nil || text == "" {
		return 0, err
	}
	return strconv.ParseInt(text, 10, 64)
}

// AddUsage adds this poll's user-inbound bytes and returns the block reason, if any.
func (h *HostedService) AddUsage(traffics []*xray.Traffic) string {
	limits, ok, err := hostedLimits()
	if !ok {
		return ""
	}
	if err != nil {
		return hosted.BlockedNoLimits
	}
	var delta int64
	for _, t := range traffics {
		if t != nil && t.IsInbound && t.Tag != "api" {
			delta += t.Up + t.Down
		}
	}
	used, err := h.usedBytes(limits)
	if err != nil {
		logger.Warning("hosted: read usage:", err)
	}
	if delta > 0 {
		used += delta
		if err := h.settingService.setString(hostedUsedKey, strconv.FormatInt(used, 10)); err != nil {
			logger.Warning("hosted: store usage:", err)
		}
	}
	return blockReason(limits, used, time.Now())
}

func blockReason(limits hosted.Limits, used int64, now time.Time) string {
	switch {
	case limits.Expired(now):
		return hosted.BlockedExpired
	case limits.TrafficBytes > 0 && used >= limits.TrafficBytes:
		return hosted.BlockedTraffic
	}
	return ""
}

// Blocked reports why this hosted panel must not serve, or "" when it may.
func (h *HostedService) Blocked() string {
	limits, ok, err := hostedLimits()
	if !ok {
		return ""
	}
	if err != nil {
		return hosted.BlockedNoLimits
	}
	used, err := h.usedBytes(limits)
	if err != nil {
		logger.Warning("hosted: read usage:", err)
	}
	return blockReason(limits, used, time.Now())
}

func (h *HostedService) View() (*HostedView, error) {
	limits, ok, err := hostedLimits()
	view := &HostedView{Enabled: ok, Limits: limits}
	if !ok {
		return view, nil
	}
	if err != nil {
		view.Blocked = hosted.BlockedNoLimits
		return view, nil
	}
	db := database.GetDB()
	if err := db.Model(&model.Inbound{}).Where("node_id IS NULL").Count(&view.Inbounds).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&model.ClientRecord{}).Count(&view.Clients).Error; err != nil {
		return nil, err
	}
	if view.UsedBytes, err = h.usedBytes(limits); err != nil {
		return nil, err
	}
	view.Blocked = blockReason(limits, view.UsedBytes, time.Now())
	return view, nil
}

// Reconcile publishes status for the master and starts or stops Xray to match the limits.
func (h *HostedService) Reconcile(xs *XrayService) {
	if !hosted.Enabled() {
		return
	}
	view, err := h.View()
	if err != nil {
		logger.Warning("hosted: status:", err)
		return
	}
	switch {
	case view.Blocked != "" && xs.IsXrayRunning():
		if err := xs.RestartXray(false); err != nil {
			logger.Info("hosted: xray stopped:", err)
		}
	case view.Blocked == "" && hostedHalted.Load():
		hostedHalted.Store(false)
		if err := xs.RestartXray(true); err != nil {
			logger.Warning("hosted: restart xray after limits cleared:", err)
		}
	}
	certFile, _ := h.settingService.GetCertFile()
	keyFile, _ := h.settingService.GetKeyFile()
	status := hosted.PanelStatus{
		WebTLS:      certFile != "" && keyFile != "",
		Inbounds:    int(view.Inbounds),
		Clients:     int(view.Clients),
		UsedBytes:   view.UsedBytes,
		Blocked:     view.Blocked,
		XrayRunning: xs.IsXrayRunning(),
		UpdatedAt:   time.Now().UnixMilli(),
	}
	if err := hosted.WriteStatus(hosted.Dir(), status); err != nil {
		logger.Warning("hosted: write status:", err)
	}
}

// SetHostedAddress applies the address the master assigned this hosted panel.
func (s *SettingService) SetHostedAddress(webPort, subPort int, basePath string) error {
	if err := s.SetListen(""); err != nil {
		return err
	}
	if err := s.SetPort(webPort); err != nil {
		return err
	}
	if err := s.SetBasePath(basePath); err != nil {
		return err
	}
	if err := s.setString("subListen", ""); err != nil {
		return err
	}
	return s.setInt("subPort", subPort)
}
