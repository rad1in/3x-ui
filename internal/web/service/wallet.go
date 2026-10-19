package service

import (
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

const walletHistoryLimit = 50

// walletMu serialises balance checks with the debits that follow them; the
// panel is the only writer of its own database.
var walletMu sync.Mutex

// WalletBalance is a customer's balance in one currency, in its minor unit.
type WalletBalance struct {
	Currency string `json:"currency" example:"USD"`
	Amount   int64  `json:"amount" example:"250"`
}

// WalletView is a customer's balances and latest ledger lines.
type WalletView struct {
	Email    string              `json:"email" example:"alice"`
	Balances []WalletBalance     `json:"balances"`
	Entries  []model.WalletEntry `json:"entries"`
}

// WalletService keeps the per-customer balances. A customer is the client with
// that username; every balance is the sum of its ledger entries.
type WalletService struct {
	clients ClientService
}

func balanceIn(tx *gorm.DB, email, currency string) (int64, error) {
	var sum int64
	err := tx.Model(&model.WalletEntry{}).Where("email = ? AND currency = ?", email, currency).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum).Error
	return sum, err
}

// Balances lists the non-zero balances of a customer.
func (s *WalletService) Balances(email string) ([]WalletBalance, error) {
	var rows []WalletBalance
	err := database.GetDB().Model(&model.WalletEntry{}).Where("email = ?", email).
		Select("currency, COALESCE(SUM(amount), 0) AS amount").Group("currency").Order("currency").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]WalletBalance, 0, len(rows))
	for _, r := range rows {
		if r.Amount != 0 {
			out = append(out, r)
		}
	}
	return out, nil
}

// View returns balances and the latest entries of a customer.
func (s *WalletService) View(email string) (*WalletView, error) {
	balances, err := s.Balances(email)
	if err != nil {
		return nil, err
	}
	view := &WalletView{Email: email, Balances: balances, Entries: []model.WalletEntry{}}
	err = database.GetDB().Where("email = ?", email).Order("id desc").Limit(walletHistoryLimit).Find(&view.Entries).Error
	return view, err
}

// Adjust is the admin credit or debit of a customer's wallet; a debit may not
// take the balance below zero.
func (s *WalletService) Adjust(email, currency string, amount int64, note string) (*model.WalletEntry, error) {
	if _, ok := CurrencyDecimals(currency); !ok {
		return nil, common.NewError("unsupported currency:", currency)
	}
	if amount == 0 {
		return nil, common.NewError("amount cannot be zero")
	}
	if _, err := s.clients.GetRecordByEmail(nil, email); err != nil {
		return nil, salesErr(ErrEmailUnknown)
	}
	walletMu.Lock()
	defer walletMu.Unlock()
	entry := &model.WalletEntry{Email: email, Currency: currency, Amount: amount, Kind: model.WalletAdmin, Note: truncate(note, maxRejectReason)}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if amount < 0 {
			balance, err := balanceIn(tx, email, currency)
			if err != nil {
				return err
			}
			if balance+amount < 0 {
				return salesErr(ErrWalletInsufficient)
			}
		}
		return tx.Create(entry).Error
	})
	return entry, err
}

// book adds an entry whose Ref is new; a Ref already booked is not an error,
// so retries and repeated webhooks never pay twice.
func bookOnce(tx *gorm.DB, entry *model.WalletEntry) error {
	res := tx.Where("ref = ?", *entry.Ref).FirstOrCreate(entry)
	return res.Error
}
