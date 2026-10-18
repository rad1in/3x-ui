package service

import (
	"crypto/rand"
	"errors"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"gorm.io/gorm"
)

// referralAlphabet leaves out the characters people misread (0/O, 1/I).
const (
	referralAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	referralLength   = 8
)

// ReferralService hands out referral codes and pays their bonuses.
type ReferralService struct {
	clients ClientService
	config  SalesConfigService
}

func newReferralCode() string {
	b := make([]byte, referralLength)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = referralAlphabet[int(b[i])%len(referralAlphabet)]
	}
	return string(b)
}

// CodeFor returns the referral code of an existing client, creating it on the
// first request.
func (s *ReferralService) CodeFor(email string) (string, error) {
	if _, err := s.clients.GetRecordByEmail(nil, email); err != nil {
		return "", salesErr(ErrEmailUnknown)
	}
	db := database.GetDB()
	var existing model.ReferralCode
	err := db.Where("email = ?", email).First(&existing).Error
	if err == nil {
		return existing.Code, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	for range 5 {
		row := model.ReferralCode{Email: email, Code: newReferralCode()}
		if err := db.Create(&row).Error; err == nil {
			return row.Code, nil
		}
		if err := db.Where("email = ?", email).First(&existing).Error; err == nil {
			return existing.Code, nil
		}
	}
	return "", errors.New("could not allocate a referral code")
}

// Referrer resolves a code to the referring client's username.
func (s *ReferralService) Referrer(code string) (string, error) {
	var row model.ReferralCode
	err := database.GetDB().Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", salesErr(ErrReferralInvalid)
	}
	return row.Email, err
}

// creditBonus pays the referrer of a delivered order their share of its price,
// in the order's currency. It books once per order and only logs on failure:
// the sale is already delivered.
func (s *ReferralService) creditBonus(order *model.Order) {
	if order.ReferrerEmail == "" {
		return
	}
	cfg, err := s.config.Get()
	if err != nil || !cfg.Referral.Enabled || cfg.Referral.BonusPercent <= 0 {
		return
	}
	bonus := order.Amount * int64(cfg.Referral.BonusPercent) / 100
	if bonus <= 0 {
		return
	}
	ref := "referral:order:" + strconv.Itoa(order.Id)
	entry := &model.WalletEntry{
		Email: order.ReferrerEmail, Currency: order.Currency, Amount: bonus, Kind: model.WalletReferral,
		Ref: &ref, OrderId: order.Id, Note: "referral bonus for " + order.ClientEmail,
	}
	walletMu.Lock()
	defer walletMu.Unlock()
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error { return bookOnce(tx, entry) }); err != nil {
		logger.Warningf("order %d: referral bonus: %v", order.Id, err)
	}
}
