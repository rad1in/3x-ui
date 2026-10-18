package service

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// cryptoExpiry is how long an unpaid crypto invoice holds its username; a
// coin transfer can take far longer than a card payment.
const cryptoExpiry = 7 * 24 * time.Hour

// HandleCryptoIPN processes a NOWPayments webhook. Nothing is trusted before
// the HMAC signature checks out, and the order is matched by its token and
// price so a signed payment for another order cannot settle this one. Only a
// finished (fully confirmed) payment delivers; a partial payment is parked for
// the admin, and a payment that arrives for an order already failed or
// canceled is parked too instead of being lost.
func (s *OrderService) HandleCryptoIPN(inboundSvc *InboundService, body []byte, signature string) (*model.Order, bool, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, false, err
	}
	if !gatewayEnabled(cfg, GatewayCrypto) {
		return nil, false, salesErr(ErrGatewayUnavailable)
	}
	if !payment.VerifyIPNSignature(cfg.Crypto.IpnSecret, body, signature) {
		return nil, false, salesErr(ErrBadSignature)
	}
	ipn, err := payment.ParseIPN(body)
	if err != nil {
		return nil, false, err
	}

	settleMu.Lock()
	defer settleMu.Unlock()
	order, err := s.GetOrderByToken(ipn.OrderID)
	if err != nil {
		return nil, false, err
	}
	if order.Gateway != GatewayCrypto {
		return nil, false, salesErr(ErrOrderState)
	}
	if !ipnMatchesOrder(ipn, order) {
		logger.Warningf("order %d: crypto IPN %s does not match the order price", order.Id, ipn.PaymentID)
		return order, false, salesErr(ErrAmountMismatch)
	}
	switch ipn.Status {
	case payment.NPFinished:
		return s.cryptoFinished(inboundSvc, order, ipn)
	case payment.NPPartiallyPaid:
		note := "partially paid: " + ipn.ActuallyPaid + " of " + ipn.PayAmount
		return s.parkForReview(order, model.OrderStatusPending, ipn.PaymentID, note)
	case payment.NPFailed, payment.NPExpired:
		closed, err := s.closeOrder(order.Id, model.OrderStatusFailed)
		return closed, false, err
	default:
		return order, false, nil
	}
}

func (s *OrderService) cryptoFinished(inboundSvc *InboundService, order *model.Order, ipn *payment.IPN) (*model.Order, bool, error) {
	switch order.Status {
	case model.OrderStatusPending, model.OrderStatusReview:
		if err := database.GetDB().Model(&model.Order{}).Where("id = ?", order.Id).Update("receipt_ref", ipn.PaymentID).Error; err != nil {
			return nil, false, err
		}
		needRestart, err := s.FulfilOrder(inboundSvc, order.Id)
		reloaded, rerr := s.getOrder(order.Id)
		if err == nil {
			err = rerr
		}
		return reloaded, needRestart, err
	case model.OrderStatusPaid:
		return s.redeliver(inboundSvc, order)
	case model.OrderStatusFailed, model.OrderStatusCanceled:
		return s.parkForReview(order, order.Status, ipn.PaymentID, "late payment after the order was "+order.Status)
	default:
		logger.Warningf("order %d: crypto payment %s finished on an order that was %s", order.Id, ipn.PaymentID, order.Status)
		return order, false, nil
	}
}

// parkForReview puts an order whose payment needs a human decision in front
// of the admin; only an order still in status from is moved.
func (s *OrderService) parkForReview(order *model.Order, from, paymentID, note string) (*model.Order, bool, error) {
	err := database.GetDB().Model(&model.Order{}).Where("id = ? AND status = ?", order.Id, from).
		Updates(map[string]any{"status": model.OrderStatusReview, "receipt_ref": paymentID, "receipt_note": note}).Error
	if err != nil {
		return nil, false, err
	}
	reloaded, err := s.getOrder(order.Id)
	return reloaded, false, err
}

func ipnMatchesOrder(ipn *payment.IPN, order *model.Order) bool {
	minor, ok := decimalToMinor(ipn.PriceAmount)
	return ok && minor == order.Amount && strings.EqualFold(ipn.PriceCurrency, order.Currency)
}

// decimalToMinor reads a price such as "19.99" as 1999 minor units; a value
// with more than two decimals is refused rather than rounded.
func decimalToMinor(text string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	cents := math.Round(f * 100)
	if math.Abs(f*100-cents) > 1e-6 {
		return 0, false
	}
	return int64(cents), true
}

// expireCryptoOrders abandons crypto orders left unpaid for a week.
func (s *OrderService) expireCryptoOrders(now time.Time) error {
	return database.GetDB().Model(&model.Order{}).
		Where("gateway = ? AND status = ? AND created_at <= ?", GatewayCrypto, model.OrderStatusPending, now.Add(-cryptoExpiry).UnixMilli()).
		Update("status", model.OrderStatusCanceled).Error
}
