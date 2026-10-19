package service

import (
	"encoding/json"
	"math/rand/v2"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

const (
	rotationPortTries      = 64
	rotationMaxInterval    = 24 * 365
	rotationDefaultMin     = 20000
	rotationDefaultMax     = 40000
	rotationDefaultSNIPort = 443
)

var rotationHostPattern = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)

// rotationMu serializes rotations so a manual run and the scheduler cannot
// pick the same port for two inbounds.
var rotationMu sync.Mutex

// localPortFree reports whether this host can bind port for the given transports.
// It is a variable so tests can stand in for a busy host.
var localPortFree = func(port int, bits transportBits) bool {
	addr := ":" + strconv.Itoa(port)
	if bits&transportTCP != 0 {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		_ = ln.Close()
	}
	if bits&transportUDP != 0 {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		_ = pc.Close()
	}
	return true
}

// RotationResult is what one rotation changed. A zero Old/New SNI means SNI was not rotated.
type RotationResult struct {
	InboundId   int    `json:"inboundId" example:"3"`
	OldPort     int    `json:"oldPort" example:"8443"`
	NewPort     int    `json:"newPort" example:"23871"`
	OldSni      string `json:"oldSni" example:"www.microsoft.com"`
	NewSni      string `json:"newSni" example:"www.apple.com"`
	NeedRestart bool   `json:"needRestart" example:"false"`
}

// InboundRotationService moves an inbound to a new port and/or REALITY SNI. The
// change is an ordinary inbound update, so a node inbound is pushed to its node.
type InboundRotationService struct {
	inbounds InboundService
}

type sniEntry struct {
	host   string
	target string
}

func parseSNIList(list string) ([]sniEntry, error) {
	fields := strings.FieldsFunc(list, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]sniEntry, 0, len(fields))
	for _, f := range fields {
		host, port := f, rotationDefaultSNIPort
		if h, p, err := net.SplitHostPort(f); err == nil {
			n, convErr := strconv.Atoi(p)
			if convErr != nil || n < 1 || n > 65535 {
				return nil, common.NewErrorf("invalid port in %q", f)
			}
			host, port = h, n
		}
		if !rotationHostPattern.MatchString(host) {
			return nil, common.NewErrorf("%q is not a domain name", f)
		}
		out = append(out, sniEntry{host: host, target: net.JoinHostPort(host, strconv.Itoa(port))})
	}
	return out, nil
}

func isRealityStream(stream string) bool {
	var s struct {
		Security string `json:"security"`
	}
	return json.Unmarshal([]byte(stream), &s) == nil && s.Security == "reality"
}

func (s *InboundRotationService) stored(inboundID int) (*model.InboundRotation, error) {
	row := &model.InboundRotation{}
	err := database.GetDB().Where("inbound_id = ?", inboundID).First(row).Error
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Get returns the saved setup, or the defaults for an inbound that has none.
func (s *InboundRotationService) Get(inboundID int) (*model.InboundRotation, error) {
	row, err := s.stored(inboundID)
	if database.IsNotFound(err) {
		return &model.InboundRotation{InboundId: inboundID, PortMin: rotationDefaultMin, PortMax: rotationDefaultMax}, nil
	}
	return row, err
}

func validateRotation(cfg *model.InboundRotation, ib *model.Inbound) error {
	if !cfg.RotatePort && !cfg.RotateSNI {
		return common.NewError("choose what to rotate: the port, the SNI, or both")
	}
	if cfg.IntervalHours < 0 || cfg.IntervalHours > rotationMaxInterval {
		return common.NewErrorf("interval must be between 0 and %d hours", rotationMaxInterval)
	}
	if cfg.RotatePort && (cfg.PortMin < 1 || cfg.PortMax > 65535 || cfg.PortMin > cfg.PortMax) {
		return common.NewError("port range must lie within 1-65535 with the minimum not above the maximum")
	}
	if cfg.RotateSNI {
		if !isRealityStream(ib.StreamSettings) {
			return common.NewError("SNI rotation works on REALITY inbounds only")
		}
		entries, err := parseSNIList(cfg.SniList)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return common.NewError("add at least one SNI to rotate through")
		}
	}
	return nil
}

// Save stores the setup of cfg.InboundId. Last-rotation fields are the
// service's own, and the schedule clock restarts only when the interval changes.
func (s *InboundRotationService) Save(cfg *model.InboundRotation) (*model.InboundRotation, error) {
	ib, err := s.inbounds.GetInbound(cfg.InboundId)
	if err != nil {
		return nil, err
	}
	if err := validateRotation(cfg, ib); err != nil {
		return nil, err
	}
	rotationMu.Lock()
	defer rotationMu.Unlock()
	row, err := s.stored(cfg.InboundId)
	if err != nil && !database.IsNotFound(err) {
		return nil, err
	}
	if row == nil {
		row = &model.InboundRotation{InboundId: cfg.InboundId}
	}
	oldInterval := row.IntervalHours
	row.RotatePort, row.PortMin, row.PortMax = cfg.RotatePort, cfg.PortMin, cfg.PortMax
	row.RotateSNI, row.SniList, row.IntervalHours = cfg.RotateSNI, cfg.SniList, cfg.IntervalHours
	switch {
	case row.IntervalHours == 0:
		row.NextRotationAt = 0
	case row.IntervalHours != oldInterval || row.NextRotationAt == 0:
		row.NextRotationAt = time.Now().Add(time.Duration(row.IntervalHours) * time.Hour).UnixMilli()
	}
	if err := database.GetDB().Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// pickPort draws random ports from the range until one passes the panel's
// conflict check and, for a local inbound, is free on this host.
func pickRotationPort(next *model.Inbound, cfg *model.InboundRotation) (int, error) {
	span := cfg.PortMax - cfg.PortMin + 1
	bits := inboundTransports(next.Protocol, next.StreamSettings, next.Settings)
	tried := map[int]bool{next.Port: true}
	for range rotationPortTries {
		candidate := cfg.PortMin + rand.IntN(span)
		if tried[candidate] {
			continue
		}
		tried[candidate] = true
		probe := *next
		probe.Port = candidate
		conflict, err := checkPortConflictTx(database.GetDB(), &probe, next.Id)
		if err != nil {
			return 0, err
		}
		if conflict != nil {
			continue
		}
		if next.NodeID == nil && !localPortFree(candidate, bits) {
			continue
		}
		return candidate, nil
	}
	return 0, common.NewErrorf("no free port found in %d-%d", cfg.PortMin, cfg.PortMax)
}

// rotateRealitySNI returns the stream settings moved to another SNI of the list,
// and the SNI it chose; serverNames is replaced by that single name.
func rotateRealitySNI(stream, list string) (string, string, string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(stream), &root); err != nil {
		return "", "", "", err
	}
	rs, _ := root["realitySettings"].(map[string]any)
	if rs == nil {
		return "", "", "", common.NewError("the inbound has no REALITY settings")
	}
	current := ""
	if names, ok := rs["serverNames"].([]any); ok && len(names) > 0 {
		current, _ = names[0].(string)
	}
	entries, err := parseSNIList(list)
	if err != nil {
		return "", "", "", err
	}
	candidates := slices.DeleteFunc(slices.Clone(entries), func(e sniEntry) bool { return e.host == current })
	if len(candidates) == 0 {
		return "", "", "", common.NewError("the SNI list has no entry other than the one in use")
	}
	chosen := candidates[rand.IntN(len(candidates))]
	rs["target"] = chosen.target
	delete(rs, "dest")
	rs["serverNames"] = []string{chosen.host}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", "", "", err
	}
	return string(out), current, chosen.host, nil
}

// Rotate applies the saved setup to the inbound now.
func (s *InboundRotationService) Rotate(inboundID int, now time.Time) (*RotationResult, error) {
	rotationMu.Lock()
	defer rotationMu.Unlock()
	cfg, err := s.stored(inboundID)
	if database.IsNotFound(err) {
		return nil, common.NewError("save a rotation setup for this inbound first")
	}
	if err != nil {
		return nil, err
	}
	ib, err := s.inbounds.GetInbound(inboundID)
	if err != nil {
		return nil, err
	}
	next := *ib
	res := &RotationResult{InboundId: inboundID, OldPort: ib.Port, NewPort: ib.Port}
	if cfg.RotatePort {
		if res.NewPort, err = pickRotationPort(&next, cfg); err != nil {
			return nil, err
		}
		next.Port = res.NewPort
	}
	if cfg.RotateSNI {
		if next.StreamSettings, res.OldSni, res.NewSni, err = rotateRealitySNI(ib.StreamSettings, cfg.SniList); err != nil {
			return nil, err
		}
	}
	if _, res.NeedRestart, err = s.inbounds.UpdateInbound(&next); err != nil {
		return nil, err
	}

	updates := map[string]any{"last_rotated_at": now.UnixMilli(), "last_port": res.NewPort, "last_sni": res.NewSni}
	if cfg.IntervalHours > 0 {
		updates["next_rotation_at"] = now.Add(time.Duration(cfg.IntervalHours) * time.Hour).UnixMilli()
	}
	if err := database.GetDB().Model(&model.InboundRotation{}).Where("id = ?", cfg.Id).Updates(updates).Error; err != nil {
		logger.Warning("inbound rotation: record rotation of inbound", inboundID, "failed:", err)
	}
	logger.Infof("inbound %d rotated: port %d -> %d, SNI %q -> %q", inboundID, res.OldPort, res.NewPort, res.OldSni, res.NewSni)
	return res, nil
}

// Tick rotates every scheduled inbound that is due and returns what it changed.
// One inbound failing never stops the others.
func (s *InboundRotationService) Tick(now time.Time) []*RotationResult {
	var due []model.InboundRotation
	err := database.GetDB().
		Where("interval_hours > 0 AND next_rotation_at > 0 AND next_rotation_at <= ?", now.UnixMilli()).
		Find(&due).Error
	if err != nil {
		logger.Warning("inbound rotation: list due rotations failed:", err)
		return nil
	}
	var out []*RotationResult
	for _, cfg := range due {
		res, err := s.Rotate(cfg.InboundId, now)
		if err != nil {
			s.deferAfterFailure(&cfg, now, err)
			continue
		}
		out = append(out, res)
	}
	return out
}

// deferAfterFailure pushes the next attempt one interval out so a rotation that
// cannot succeed (no free port, inbound gone) neither retries every tick nor
// stays due forever.
func (s *InboundRotationService) deferAfterFailure(cfg *model.InboundRotation, now time.Time, cause error) {
	logger.Warning("inbound rotation: inbound", cfg.InboundId, "not rotated:", cause)
	next := now.Add(time.Duration(cfg.IntervalHours) * time.Hour).UnixMilli()
	if err := database.GetDB().Model(&model.InboundRotation{}).Where("id = ?", cfg.Id).Update("next_rotation_at", next).Error; err != nil {
		logger.Warning("inbound rotation: defer inbound", cfg.InboundId, "failed:", err)
	}
}

// dropInboundRotation removes the rotation setup of a deleted inbound.
func dropInboundRotation(tx *gorm.DB, inboundID int) error {
	return tx.Where("inbound_id = ?", inboundID).Delete(&model.InboundRotation{}).Error
}
