package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"

	"gorm.io/gorm"
)

const maxOrderPage = 200

// Payment methods an order can use.
const (
	GatewayCard   = "card"
	GatewayWallet = "wallet"
)

// Error codes of SalesError. They double as the keys the public shop page
// translates, so they must stay stable.
const (
	ErrPlanUnavailable     = "plan_unavailable"
	ErrInvalidEmail        = "invalid_email"
	ErrInvalidKind         = "invalid_kind"
	ErrEmailTaken          = "email_taken"
	ErrEmailUnknown        = "email_unknown"
	ErrGatewayUnavailable  = "gateway_unavailable"
	ErrOrderNotFound       = "order_not_found"
	ErrOrderState          = "order_state"
	ErrReceiptInvalid      = "receipt_invalid"
	ErrCurrencyUnsupported = "currency_unsupported"
	ErrGatewayFailed       = "gateway_failed"
	ErrBadSignature        = "bad_signature"
	ErrDiscountInvalid     = "discount_invalid"
	ErrDiscountUsed        = "discount_used"
	ErrReferralInvalid     = "referral_invalid"
	ErrWalletInsufficient  = "wallet_insufficient"
	ErrProofInvalid        = "proof_invalid"
	ErrAmountMismatch      = "amount_mismatch"
)

// SalesError is a refusal that is safe to show a customer verbatim by code.
type SalesError struct{ Code string }

func (e *SalesError) Error() string { return e.Code }

func salesErr(code string) error { return &SalesError{Code: code} }

// OrderPage is one page of orders, newest first, with the unpaged total.
type OrderPage struct {
	Items []model.Order `json:"items"`
	Total int64         `json:"total"`
}

// CreateOrderInput is what a customer submits to start a purchase.
type CreateOrderInput struct {
	PlanId  int    `json:"planId"`
	Kind    string `json:"kind"`
	Email   string `json:"email"`
	Contact string `json:"contact"`
	Gateway string `json:"gateway"`
	// DiscountCode and ReferralCode are optional coupons. Proof is the client's
	// subscription id, which a wallet payment needs to show the account is theirs.
	DiscountCode string `json:"discountCode"`
	ReferralCode string `json:"referralCode"`
	Proof        string `json:"proof"`
	// TgId is set only by the Telegram bot, which has already identified the
	// buyer; it must never be bindable from the public API.
	TgId int64 `json:"-"`
}

// shopUsername is the pattern a new customer's username must match; it is
// stricter than the panel's own email rule so look-alike names cannot be bought.
var shopUsername = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

const (
	maxContactLen     = 100
	maxReceiptRefLen  = 64
	maxReceiptNoteLen = 500
	maxRejectReason   = 300
)

// OrderService runs the purchase lifecycle of plans.
type OrderService struct {
	plans     PlanService
	config    SalesConfigService
	clients   ClientService
	referrals ReferralService
	// gatewayFactory replaces the real payment clients in tests.
	gatewayFactory func(name string, cfg *SalesConfig) payment.Starter
}

// orderCreateMu makes the coupon use count, the wallet balance check and the
// insert of a new order one step.
var orderCreateMu sync.Mutex

// fulfilMu serialises approvals so one order never provisions twice; the
// panel is the only writer of its own database.
var fulfilMu sync.Mutex

// ListOrders pages orders newest first; an empty status lists every status.
func (s *OrderService) ListOrders(status string, limit, offset int) (*OrderPage, error) {
	if limit <= 0 || limit > maxOrderPage {
		limit = maxOrderPage
	}
	if offset < 0 {
		offset = 0
	}
	q := database.GetDB().Model(&model.Order{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	page := &OrderPage{Items: []model.Order{}}
	if err := q.Count(&page.Total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("id desc").Limit(limit).Offset(offset).Find(&page.Items).Error; err != nil {
		return nil, err
	}
	return page, nil
}

// GetOrderByToken returns the order a customer holds the token of.
func (s *OrderService) GetOrderByToken(token string) (*model.Order, error) {
	order := &model.Order{}
	err := database.GetDB().Where("token = ?", token).First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || token == "" {
		return nil, salesErr(ErrOrderNotFound)
	}
	return order, err
}

// GetOrder returns an order by its admin id.
func (s *OrderService) GetOrder(id int) (*model.Order, error) { return s.getOrder(id) }

func (s *OrderService) getOrder(id int) (*model.Order, error) {
	order := &model.Order{}
	err := database.GetDB().First(order, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, salesErr(ErrOrderNotFound)
	}
	return order, err
}

func gatewayEnabled(cfg *SalesConfig, gateway string) bool {
	switch gateway {
	case GatewayCard:
		return cfg.Card.Enabled
	case GatewayWallet:
		return cfg.WalletEnabled
	case GatewayZarinpal:
		return cfg.Zarinpal.Enabled && cfg.Zarinpal.MerchantId != "" && cfg.PublicBaseURL != ""
	case GatewayIDPay:
		return cfg.IDPay.Enabled && cfg.IDPay.ApiKey != "" && cfg.PublicBaseURL != ""
	case GatewayCrypto:
		return cfg.Crypto.Enabled && cfg.Crypto.ApiKey != "" && cfg.Crypto.IpnSecret != "" && cfg.PublicBaseURL != ""
	default:
		return false
	}
}

// CreateOrder opens a pending order for an enabled plan on an enabled payment
// method. A new purchase needs a free username, a renewal an existing client.
func (s *OrderService) CreateOrder(in CreateOrderInput) (*model.Order, error) {
	if in.Kind != model.OrderKindNew && in.Kind != model.OrderKindRenew {
		return nil, salesErr(ErrInvalidKind)
	}
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	if !gatewayEnabled(cfg, in.Gateway) || (in.Gateway == GatewayWallet && in.Kind != model.OrderKindRenew) {
		return nil, salesErr(ErrGatewayUnavailable)
	}
	orderCreateMu.Lock()
	defer orderCreateMu.Unlock()
	plan, err := s.plans.GetPlan(in.PlanId)
	if err != nil || !plan.Enabled {
		return nil, salesErr(ErrPlanUnavailable)
	}
	if !gatewayAcceptsCurrency(in.Gateway, plan.Currency) {
		return nil, salesErr(ErrCurrencyUnsupported)
	}
	email := strings.TrimSpace(in.Email)
	if in.Kind == model.OrderKindNew {
		email = strings.ToLower(email)
		if !shopUsername.MatchString(email) {
			return nil, salesErr(ErrInvalidEmail)
		}
	} else if email == "" || len(email) > 128 || validateClientEmail(email) != nil {
		return nil, salesErr(ErrInvalidEmail)
	}
	if err := s.checkUsername(in.Kind, email); err != nil {
		return nil, err
	}
	if err := s.checkTelegramOwner(in, email); err != nil {
		return nil, err
	}
	referrer, err := s.referrerFor(cfg, in, email)
	if err != nil {
		return nil, err
	}
	price, err := priceOrder(plan, in.DiscountCode, email)
	if err != nil {
		return nil, err
	}
	order := &model.Order{
		Token:        randomOrderToken(),
		PlanId:       plan.Id,
		PlanName:     plan.Name,
		PlanTemplate: plan.TemplateId,
		TotalGB:      plan.TotalGB,
		DurationDays: plan.DurationDays,
		LimitIP:      plan.LimitIP,
		Kind:         in.Kind,
		ClientEmail:  email,
		Contact:      truncate(strings.TrimSpace(in.Contact), maxContactLen),
		TgId:         in.TgId,
		Amount:       price.Final,
		ListAmount:   price.List,
		Currency:     plan.Currency,
		Gateway:      in.Gateway,
		Status:       model.OrderStatusPending,
	}
	if price.Code != nil {
		order.DiscountCode, order.DiscountId = price.Code.Code, price.Code.Id
	}
	if referrer != "" {
		order.ReferralCode, order.ReferrerEmail = normalizeCode(in.ReferralCode), referrer
	}
	if in.Gateway == GatewayWallet {
		return order, s.createWalletOrder(order, in.Proof)
	}
	if err := database.GetDB().Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// referrerFor resolves the referral code of a first purchase; it is ignored
// while referrals are off and on renewals.
func (s *OrderService) referrerFor(cfg *SalesConfig, in CreateOrderInput, email string) (string, error) {
	if !cfg.Referral.Enabled || in.Kind != model.OrderKindNew || strings.TrimSpace(in.ReferralCode) == "" {
		return "", nil
	}
	who, err := s.referrals.Referrer(in.ReferralCode)
	if err != nil {
		return "", err
	}
	if who == email {
		return "", salesErr(ErrReferralInvalid)
	}
	return who, nil
}

// createWalletOrder stores a wallet order and takes its price out of the
// balance in one transaction. The client's subscription id is the proof that
// the account belongs to whoever is paying from its wallet.
func (s *OrderService) createWalletOrder(order *model.Order, proof string) error {
	rec, err := s.clients.GetRecordByEmail(nil, order.ClientEmail)
	if err != nil {
		return salesErr(ErrEmailUnknown)
	}
	ownedByBuyer := order.TgId != 0 && rec.TgID == order.TgId
	if !ownedByBuyer && (proof == "" || subtle.ConstantTimeCompare([]byte(rec.SubID), []byte(proof)) != 1) {
		return salesErr(ErrProofInvalid)
	}
	walletMu.Lock()
	defer walletMu.Unlock()
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		balance, err := balanceIn(tx, order.ClientEmail, order.Currency)
		if err != nil {
			return err
		}
		if balance < order.Amount {
			return salesErr(ErrWalletInsufficient)
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		ref := "spend:order:" + strconv.Itoa(order.Id)
		return tx.Create(&model.WalletEntry{
			Email: order.ClientEmail, Currency: order.Currency, Amount: -order.Amount,
			Kind: model.WalletSpend, Ref: &ref, OrderId: order.Id,
		}).Error
	})
}

// Checkout is a customer's purchase from start to the point where they must
// act: an online payment is opened at its gateway, a wallet payment is
// delivered at once, a card order waits for its receipt.
func (s *OrderService) Checkout(inboundSvc *InboundService, in CreateOrderInput) (*model.Order, bool, error) {
	order, err := s.CreateOrder(in)
	if err != nil {
		return nil, false, err
	}
	switch {
	case isOnline(order.Gateway):
		order, err = s.StartPayment(order)
		return order, false, err
	case order.Gateway == GatewayWallet:
		needRestart, err := s.FulfilOrder(inboundSvc, order.Id)
		if err != nil {
			logger.Warningf("wallet order %d is paid but was not delivered: %v", order.Id, err)
		}
		reloaded, rerr := s.getOrder(order.Id)
		return reloaded, needRestart, rerr
	default:
		return order, false, nil
	}
}

// QuoteInput asks what a plan would cost with a coupon.
type QuoteInput struct {
	PlanId       int    `json:"planId"`
	DiscountCode string `json:"discountCode"`
	Email        string `json:"email"`
}

// Quote is a price preview; nothing is reserved.
type Quote struct {
	List         int64  `json:"list" example:"250000"`
	Discount     int64  `json:"discount" example:"25000"`
	Final        int64  `json:"final" example:"225000"`
	Currency     string `json:"currency" example:"IRT"`
	DiscountCode string `json:"discountCode" example:"WELCOME10"`
}

func (s *OrderService) Quote(in QuoteInput) (*Quote, error) {
	plan, err := s.plans.GetPlan(in.PlanId)
	if err != nil || !plan.Enabled {
		return nil, salesErr(ErrPlanUnavailable)
	}
	price, err := priceOrder(plan, in.DiscountCode, strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil {
		return nil, err
	}
	quote := &Quote{List: price.List, Discount: price.Discount, Final: price.Final, Currency: plan.Currency}
	if price.Code != nil {
		quote.DiscountCode = price.Code.Code
	}
	return quote, nil
}

// checkUsername enforces the new/renew rule against clients and open orders.
func (s *OrderService) checkUsername(kind, email string) error {
	db := database.GetDB()
	var clients int64
	if err := db.Model(&model.ClientRecord{}).Where("email = ?", email).Count(&clients).Error; err != nil {
		return err
	}
	if kind == model.OrderKindRenew {
		if clients == 0 {
			return salesErr(ErrEmailUnknown)
		}
		return nil
	}
	if clients > 0 {
		return salesErr(ErrEmailTaken)
	}
	var open int64
	err := db.Model(&model.Order{}).
		Where("client_email = ? AND kind = ?", email, model.OrderKindNew).
		Where("status IN ? OR (status = ? AND fulfilled_at = 0)", []string{model.OrderStatusPending, model.OrderStatusReview}, model.OrderStatusPaid).
		Count(&open).Error
	if err != nil {
		return err
	}
	if open > 0 {
		return salesErr(ErrEmailTaken)
	}
	return nil
}

// SubmitReceipt records the customer's card transfer reference and puts the
// order in front of the admin.
func (s *OrderService) SubmitReceipt(token, ref, note string) (*model.Order, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > maxReceiptRefLen {
		return nil, salesErr(ErrReceiptInvalid)
	}
	order, err := s.GetOrderByToken(token)
	if err != nil {
		return nil, err
	}
	res := database.GetDB().Model(&model.Order{}).
		Where("id = ? AND gateway = ? AND status IN ?", order.Id, GatewayCard,
			[]string{model.OrderStatusPending, model.OrderStatusReview}).
		Updates(map[string]any{
			"status":       model.OrderStatusReview,
			"receipt_ref":  ref,
			"receipt_note": truncate(strings.TrimSpace(note), maxReceiptNoteLen),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, salesErr(ErrOrderState)
	}
	return s.getOrder(order.Id)
}

// RejectOrder closes an order that has not been paid for good; a wallet order
// gets its money back.
func (s *OrderService) RejectOrder(id int, reason string) (*model.Order, error) {
	res := database.GetDB().Model(&model.Order{}).
		Where("id = ? AND status IN ?", id, []string{model.OrderStatusPending, model.OrderStatusReview}).
		Updates(map[string]any{
			"status":        model.OrderStatusRejected,
			"reject_reason": truncate(strings.TrimSpace(reason), maxRejectReason),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		if _, err := s.getOrder(id); err != nil {
			return nil, err
		}
		return nil, salesErr(ErrOrderState)
	}
	order, err := s.getOrder(id)
	if err == nil && order.Gateway == GatewayWallet {
		err = s.refundWallet(order)
	}
	return order, err
}

// refundWallet gives a wallet order's price back, once.
func (s *OrderService) refundWallet(order *model.Order) error {
	ref := "refund:order:" + strconv.Itoa(order.Id)
	entry := &model.WalletEntry{
		Email: order.ClientEmail, Currency: order.Currency, Amount: order.Amount, Kind: model.WalletRefund,
		Ref: &ref, OrderId: order.Id,
	}
	walletMu.Lock()
	defer walletMu.Unlock()
	return database.GetDB().Transaction(func(tx *gorm.DB) error { return bookOnce(tx, entry) })
}

// FulfilOrder accepts the payment of a pending or in-review order and creates
// or renews its client; on a paid order whose delivery failed it retries the
// delivery. The returned flag asks the caller to restart Xray.
func (s *OrderService) FulfilOrder(inboundSvc *InboundService, id int) (bool, error) {
	fulfilMu.Lock()
	defer fulfilMu.Unlock()

	order, err := s.getOrder(id)
	if err != nil {
		return false, err
	}
	open := order.Status == model.OrderStatusPending || order.Status == model.OrderStatusReview
	retry := order.Status == model.OrderStatusPaid && order.FulfilledAt == 0
	if !open && !retry {
		return false, salesErr(ErrOrderState)
	}
	db := database.GetDB()
	if open {
		now := time.Now().UnixMilli()
		if err := db.Model(order).Updates(map[string]any{"status": model.OrderStatusPaid, "paid_at": now}).Error; err != nil {
			return false, err
		}
	}
	needRestart, err := s.deliver(inboundSvc, order)
	if err != nil {
		logger.Warningf("order %d is paid but its client was not delivered: %v", order.Id, err)
		return needRestart, err
	}
	if err := db.Model(order).Update("fulfilled_at", time.Now().UnixMilli()).Error; err != nil {
		return needRestart, err
	}
	s.referrals.creditBonus(order)
	return needRestart, nil
}

// randomOrderToken is the unguessable handle a customer uses to follow an order.
func randomOrderToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
