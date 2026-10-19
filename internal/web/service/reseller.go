package service

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

var (
	ErrResellerInsufficient = errors.New("reseller credit is insufficient")
	ErrResellerUnpriced     = errors.New("a priced reseller cannot create unlimited traffic or time")
	ErrResellerNotFound     = errors.New("reseller not found")
)

const (
	resellerLedgerLimit = 100
	resellerGiB         = int64(1) << 30
	resellerDayMs       = int64(24 * time.Hour / time.Millisecond)
)

// resellerMu serialises credit checks with the debits that follow them.
var resellerMu sync.Mutex

// ResellerService keeps the credit of resellers, the admins with the operator
// role, and bills what they create and renew by a per-GiB and per-day price list.
type ResellerService struct {
	settings SettingService
}

// ResellerPrices is a price list in credit units; a zero price makes that dimension free.
type ResellerPrices struct {
	PerGB  int64 `json:"pricePerGb" example:"100"`
	PerDay int64 `json:"pricePerDay" example:"10"`
}

// ClientTerms is what a client is sold for: TotalGB bytes (0 = unlimited) and an
// expiry in unix ms (0 = never, negative = that many ms after first use).
type ClientTerms struct {
	TotalGB    int64
	ExpiryTime int64
}

// TermsChange is the terms a client should hold once an operation succeeds.
type TermsChange struct {
	Email string
	After ClientTerms
}

// ResellerConfig is the panel's wholesale price list, which every reseller
// without its own price follows, and the credit level that raises an alert.
type ResellerConfig struct {
	PricePerGB  int64 `json:"pricePerGb" example:"100"`
	PricePerDay int64 `json:"pricePerDay" example:"10"`
	LowCredit   int64 `json:"lowCredit" example:"1000"`
}

const resellerConfigKey = "resellerConfig"

// Config returns the stored wholesale price list and alert level.
func (s *ResellerService) Config() (ResellerConfig, error) {
	var cfg ResellerConfig
	raw, err := s.settings.getString(resellerConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return cfg, err
	}
	return cfg, json.Unmarshal([]byte(raw), &cfg)
}

// SaveConfig stores the wholesale price list and alert level.
func (s *ResellerService) SaveConfig(cfg ResellerConfig) error {
	if cfg.PricePerGB < 0 || cfg.PricePerDay < 0 || cfg.LowCredit < 0 {
		return errors.New("prices and the alert level cannot be negative")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settings.saveSetting(resellerConfigKey, string(raw))
}

// Prices returns the price list the reseller is billed by: its own where set,
// the panel's list otherwise.
func (s *ResellerService) Prices(u *model.User) (ResellerPrices, error) {
	cfg, err := s.Config()
	if err != nil {
		return ResellerPrices{}, err
	}
	p := ResellerPrices{PerGB: cfg.PricePerGB, PerDay: cfg.PricePerDay}
	if u.PricePerGB != nil {
		p.PerGB = max(*u.PricePerGB, 0)
	}
	if u.PricePerDay != nil {
		p.PerDay = max(*u.PricePerDay, 0)
	}
	return p, nil
}

func isReseller(u *model.User) bool {
	return u != nil && u.Role == model.AdminRoleOperator
}

func remainingDays(expiry, now int64) int64 {
	switch {
	case expiry > 0:
		if expiry <= now {
			return 0
		}
		return (expiry - now + resellerDayMs - 1) / resellerDayMs
	case expiry < 0:
		return (-expiry + resellerDayMs - 1) / resellerDayMs
	}
	return 0
}

// termsDelta is the traffic and days a client gains going from before to after.
// The unlimited flags report a dimension that becomes unlimited.
func termsDelta(before ClientTerms, existed bool, after ClientTerms, now int64) (bytes, days int64, unlimitedTraffic, unlimitedTime bool) {
	switch {
	case after.TotalGB <= 0:
		unlimitedTraffic = !existed || before.TotalGB > 0
	case existed && before.TotalGB <= 0:
	default:
		bytes = max(after.TotalGB-max(before.TotalGB, 0), 0)
	}
	switch {
	case after.ExpiryTime == 0:
		unlimitedTime = !existed || before.ExpiryTime != 0
	case existed && before.ExpiryTime == 0:
	default:
		base := int64(0)
		if existed {
			base = remainingDays(before.ExpiryTime, now)
		}
		days = max(remainingDays(after.ExpiryTime, now)-base, 0)
	}
	return bytes, days, unlimitedTraffic, unlimitedTime
}

// priceOf is the credit units for bytes and days, rounded up; ok is false when it does not fit an int64.
func priceOf(p ResellerPrices, bytes, days int64) (int64, bool) {
	traffic := new(big.Int).Mul(big.NewInt(bytes), big.NewInt(p.PerGB))
	traffic.Add(traffic, big.NewInt(resellerGiB-1))
	traffic.Div(traffic, big.NewInt(resellerGiB))
	total := traffic.Add(traffic, new(big.Int).Mul(big.NewInt(days), big.NewInt(p.PerDay)))
	if !total.IsInt64() {
		return math.MaxInt64, false
	}
	return total.Int64(), true
}

type resellerLine struct {
	email   string
	before  ClientTerms
	existed bool
	entryId int
}

// ResellerReservation holds the credit taken for one operation. Settle returns
// what the operation did not use; a nil reservation settles to nothing.
type ResellerReservation struct {
	svc    *ResellerService
	prices ResellerPrices
	userId int
	now    int64
	lines  []resellerLine
}

func clientTermsByEmail(tx *gorm.DB, emails []string) (map[string]ClientTerms, error) {
	out := make(map[string]ClientTerms, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []model.ClientRecord
		if err := tx.Select("email", "total_gb", "expiry_time").Where("email IN ?", batch).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.Email] = ClientTerms{TotalGB: r.TotalGB, ExpiryTime: r.ExpiryTime}
		}
	}
	return out, nil
}

// Reserve debits the price of every change in one step. It returns nil for an
// actor who is not a reseller. Call Settle once the operation has run.
func (s *ResellerService) Reserve(actor *model.User, changes []TermsChange) (*ResellerReservation, error) {
	if !isReseller(actor) {
		return nil, nil
	}
	prices, err := s.Prices(actor)
	if err != nil {
		return nil, err
	}
	res := &ResellerReservation{svc: s, prices: prices, userId: actor.Id, now: time.Now().UnixMilli()}
	if res.prices == (ResellerPrices{}) {
		return res, nil
	}
	emails := make([]string, 0, len(changes))
	seen := make(map[string]struct{}, len(changes))
	unique := changes[:0:0]
	for _, c := range changes {
		if _, dup := seen[c.Email]; dup {
			continue
		}
		seen[c.Email] = struct{}{}
		emails = append(emails, c.Email)
		unique = append(unique, c)
	}
	changes = unique
	resellerMu.Lock()
	defer resellerMu.Unlock()
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		before, err := clientTermsByEmail(tx, emails)
		if err != nil {
			return err
		}
		var total int64
		entries := make([]*model.ResellerEntry, 0, len(changes))
		for _, c := range changes {
			prev, existed := before[c.Email]
			bytes, days, unlimitedTraffic, unlimitedTime := termsDelta(prev, existed, c.After, res.now)
			if (unlimitedTraffic && res.prices.PerGB > 0) || (unlimitedTime && res.prices.PerDay > 0) {
				return ErrResellerUnpriced
			}
			cost, ok := priceOf(res.prices, bytes, days)
			if !ok {
				return ErrResellerInsufficient
			}
			line := resellerLine{email: c.Email, before: prev, existed: existed}
			if cost > 0 {
				total += cost
				if total < 0 {
					return ErrResellerInsufficient
				}
				entry := &model.ResellerEntry{UserId: actor.Id, Amount: -cost, Kind: entryKind(existed), ClientEmail: c.Email, Bytes: bytes, Days: days}
				entries = append(entries, entry)
			}
			res.lines = append(res.lines, line)
		}
		if total == 0 {
			return nil
		}
		balance, err := balanceOfUser(tx, actor.Id)
		if err != nil {
			return err
		}
		if balance < total {
			return ErrResellerInsufficient
		}
		if err := tx.Create(&entries).Error; err != nil {
			return err
		}
		byEmail := make(map[string]int, len(entries))
		for _, e := range entries {
			byEmail[e.ClientEmail] = e.Id
		}
		for i := range res.lines {
			res.lines[i].entryId = byEmail[res.lines[i].email]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// PlanAdjust is the terms a bulk adjustment would leave the named clients with;
// like the adjustment itself it leaves unlimited traffic and time alone.
func (s *ResellerService) PlanAdjust(emails []string, addDays int, addBytes int64) ([]TermsChange, error) {
	before, err := clientTermsByEmail(database.GetDB(), emails)
	if err != nil {
		return nil, err
	}
	addMs := int64(addDays) * resellerDayMs
	out := make([]TermsChange, 0, len(before))
	for _, email := range emails {
		terms, ok := before[email]
		if !ok {
			continue
		}
		if addBytes != 0 && terms.TotalGB > 0 {
			terms.TotalGB += addBytes
		}
		switch {
		case addDays == 0:
		case terms.ExpiryTime > 0:
			terms.ExpiryTime += addMs
		case terms.ExpiryTime < 0:
			terms.ExpiryTime -= addMs
		}
		out = append(out, TermsChange{Email: email, After: terms})
	}
	return out, nil
}

func entryKind(existed bool) string {
	if existed {
		return model.ResellerRenew
	}
	return model.ResellerCreate
}

// Settle re-reads the clients and keeps only the part of the reserved price that
// the operation really used; an operation that did nothing is refunded in full.
func (r *ResellerReservation) Settle() error {
	if r == nil {
		return nil
	}
	var spent int64
	err := r.settle(&spent)
	if err == nil {
		r.svc.alertIfLow(r.userId, spent)
	}
	return err
}

func (r *ResellerReservation) settle(spent *int64) error {
	resellerMu.Lock()
	defer resellerMu.Unlock()
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		*spent = 0
		emails := make([]string, 0, len(r.lines))
		for _, l := range r.lines {
			if l.entryId != 0 {
				emails = append(emails, l.email)
			}
		}
		if len(emails) == 0 {
			return nil
		}
		now, err := clientTermsByEmail(tx, emails)
		if err != nil {
			return err
		}
		for _, l := range r.lines {
			if l.entryId == 0 {
				continue
			}
			after, present := now[l.email]
			var bytes, days int64
			if present {
				bytes, days, _, _ = termsDelta(l.before, l.existed, after, r.now)
			}
			cost, _ := priceOf(r.prices, bytes, days)
			*spent += cost
			if cost == 0 {
				if err := tx.Delete(&model.ResellerEntry{}, l.entryId).Error; err != nil {
					return err
				}
				continue
			}
			err := tx.Model(&model.ResellerEntry{}).Where("id = ?", l.entryId).
				Updates(map[string]any{"amount": -cost, "bytes": bytes, "days": days}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// alertIfLow publishes the low-credit alert when spending just took the balance
// below the alert level, so a reseller already below it is not alerted again.
func (s *ResellerService) alertIfLow(userId int, spent int64) {
	cfg, err := s.Config()
	if err != nil || cfg.LowCredit <= 0 || spent <= 0 || eventBus == nil {
		return
	}
	balance, err := s.Balance(userId)
	if err != nil || balance >= cfg.LowCredit || balance+spent < cfg.LowCredit {
		return
	}
	var user model.User
	if err := database.GetDB().Select("username").Where("id = ?", userId).First(&user).Error; err != nil {
		return
	}
	eventBus.Publish(eventbus.Event{
		Type:      eventbus.EventResellerCreditLow,
		Source:    user.Username,
		Data:      &eventbus.ResellerCreditData{Username: user.Username, Balance: balance, Threshold: cfg.LowCredit},
		Timestamp: time.Now(),
	})
}

func balanceOfUser(tx *gorm.DB, userId int) (int64, error) {
	var sum int64
	err := tx.Model(&model.ResellerEntry{}).Where("user_id = ?", userId).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum).Error
	return sum, err
}

// Balance is the reseller's credit.
func (s *ResellerService) Balance(userId int) (int64, error) {
	return balanceOfUser(database.GetDB(), userId)
}

func resellerUser(tx *gorm.DB, id int) (*model.User, error) {
	var u model.User
	if err := tx.Where("id = ? AND role = ?", id, model.AdminRoleOperator).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResellerNotFound
		}
		return nil, err
	}
	return &u, nil
}

// Topup adds credit to a reseller; a negative amount takes credit back but never below zero.
func (s *ResellerService) Topup(userId int, amount int64, note string) (*model.ResellerEntry, error) {
	if amount == 0 {
		return nil, errors.New("amount cannot be zero")
	}
	resellerMu.Lock()
	defer resellerMu.Unlock()
	entry := &model.ResellerEntry{UserId: userId, Amount: amount, Kind: model.ResellerTopup, Note: truncate(strings.TrimSpace(note), maxRejectReason)}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if _, err := resellerUser(tx, userId); err != nil {
			return err
		}
		if amount < 0 {
			balance, err := balanceOfUser(tx, userId)
			if err != nil {
				return err
			}
			if balance+amount < 0 {
				return ErrResellerInsufficient
			}
		}
		return tx.Create(entry).Error
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// SetPrices stores a reseller's own price list; nil clears one price back to the panel's list.
func (s *ResellerService) SetPrices(userId int, perGB, perDay *int64) error {
	if (perGB != nil && *perGB < 0) || (perDay != nil && *perDay < 0) {
		return errors.New("a price cannot be negative")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if _, err := resellerUser(tx, userId); err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userId).
			Select("price_per_gb", "price_per_day").
			Updates(&model.User{PricePerGB: perGB, PricePerDay: perDay}).Error
	})
}

// ResellerAccount is a reseller's standing and latest ledger lines.
type ResellerAccount struct {
	UserId  int                   `json:"userId" example:"2"`
	Balance int64                 `json:"balance" example:"8500"`
	Prices  ResellerPrices        `json:"prices"`
	Entries []model.ResellerEntry `json:"entries"`
}

// Account returns the credit, price list and latest ledger lines of a reseller.
func (s *ResellerService) Account(u *model.User) (*ResellerAccount, error) {
	prices, err := s.Prices(u)
	if err != nil {
		return nil, err
	}
	acc := &ResellerAccount{UserId: u.Id, Prices: prices, Entries: []model.ResellerEntry{}}
	if acc.Balance, err = s.Balance(u.Id); err != nil {
		return nil, err
	}
	err = database.GetDB().Where("user_id = ?", u.Id).Order("id desc").Limit(resellerLedgerLimit).Find(&acc.Entries).Error
	return acc, err
}

// ResellerReportRow is one reseller's sales in the reported window. Spent is
// credit used on clients; ToppedUp is credit added by the owner.
type ResellerReportRow struct {
	UserId   int    `json:"userId" example:"2"`
	Username string `json:"username" example:"shop-a"`
	Balance  int64  `json:"balance" example:"8500"`
	Created  int64  `json:"created" example:"12"`
	Renewed  int64  `json:"renewed" example:"7"`
	Spent    int64  `json:"spent" example:"15000"`
	ToppedUp int64  `json:"toppedUp" example:"20000"`
	Bytes    int64  `json:"bytes" example:"1073741824000"`
	Days     int64  `json:"days" example:"570"`
}

// ResellerReport lists every reseller, or one, with sales between from and to (unix ms, 0 = open).
type ResellerReport struct {
	Rows []ResellerReportRow `json:"rows"`
}

// Report sums the ledger per reseller; userId limits it to one reseller.
func (s *ResellerService) Report(from, to int64, userId *int) (*ResellerReport, error) {
	db := database.GetDB()
	var users []model.User
	q := db.Where("role = ?", model.AdminRoleOperator).Order("id ASC")
	if userId != nil {
		q = q.Where("id = ?", *userId)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil, err
	}
	rows := make([]ResellerReportRow, 0, len(users))
	index := make(map[int]int, len(users))
	ids := make([]int, 0, len(users))
	for _, u := range users {
		index[u.Id] = len(rows)
		ids = append(ids, u.Id)
		rows = append(rows, ResellerReportRow{UserId: u.Id, Username: u.Username})
	}
	if len(ids) == 0 {
		return &ResellerReport{Rows: rows}, nil
	}
	var balances []struct {
		UserId int
		Sum    int64
	}
	if err := db.Model(&model.ResellerEntry{}).Where("user_id IN ?", ids).Group("user_id").
		Select("user_id, COALESCE(SUM(amount), 0) AS sum").Scan(&balances).Error; err != nil {
		return nil, err
	}
	for _, b := range balances {
		rows[index[b.UserId]].Balance = b.Sum
	}
	win := db.Model(&model.ResellerEntry{}).Where("user_id IN ?", ids)
	if from > 0 {
		win = win.Where("created_at >= ?", from)
	}
	if to > 0 {
		win = win.Where("created_at <= ?", to)
	}
	var sums []struct {
		UserId int
		Kind   string
		N      int64
		Amount int64
		Bytes  int64
		Days   int64
	}
	if err := win.Group("user_id, kind").
		Select("user_id, kind, COUNT(*) AS n, COALESCE(SUM(amount), 0) AS amount, COALESCE(SUM(bytes), 0) AS bytes, COALESCE(SUM(days), 0) AS days").
		Scan(&sums).Error; err != nil {
		return nil, err
	}
	for _, g := range sums {
		row := &rows[index[g.UserId]]
		switch g.Kind {
		case model.ResellerTopup:
			row.ToppedUp += g.Amount
		case model.ResellerCreate:
			row.Created += g.N
			row.Spent -= g.Amount
			row.Bytes += g.Bytes
			row.Days += g.Days
		case model.ResellerRenew:
			row.Renewed += g.N
			row.Spent -= g.Amount
			row.Bytes += g.Bytes
			row.Days += g.Days
		}
	}
	return &ResellerReport{Rows: rows}, nil
}
