package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"gorm.io/gorm"
)

const maxOrderPage = 200

// Payment methods an order can use.
const (
	GatewayCard = "card"
)

// Error codes of SalesError. They double as the keys the public shop page
// translates, so they must stay stable.
const (
	ErrPlanUnavailable    = "plan_unavailable"
	ErrInvalidEmail       = "invalid_email"
	ErrInvalidKind        = "invalid_kind"
	ErrEmailTaken         = "email_taken"
	ErrEmailUnknown       = "email_unknown"
	ErrGatewayUnavailable = "gateway_unavailable"
	ErrOrderNotFound      = "order_not_found"
	ErrOrderState         = "order_state"
	ErrReceiptInvalid     = "receipt_invalid"
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
	plans   PlanService
	config  SalesConfigService
	clients ClientService
}

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
	if !gatewayEnabled(cfg, in.Gateway) {
		return nil, salesErr(ErrGatewayUnavailable)
	}
	plan, err := s.plans.GetPlan(in.PlanId)
	if err != nil || !plan.Enabled {
		return nil, salesErr(ErrPlanUnavailable)
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
		Amount:       plan.Price,
		Currency:     plan.Currency,
		Gateway:      in.Gateway,
		Status:       model.OrderStatusPending,
	}
	if err := database.GetDB().Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
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

// RejectOrder closes an order that has not been paid for good.
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
	return s.getOrder(id)
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
	return needRestart, db.Model(order).Update("fulfilled_at", time.Now().UnixMilli()).Error
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
