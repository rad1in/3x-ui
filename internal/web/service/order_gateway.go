package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// Online payment methods an order can use.
const (
	GatewayZarinpal = payment.Zarinpal
	GatewayIDPay    = payment.IDPay
	GatewayCrypto   = payment.NOWPayments
)

const (
	// A fresh order is left to the customer's own redirect before the
	// reconciler asks the gateway about it.
	reconcileMinAge = 2 * time.Minute
	// An order unpaid this long is abandoned.
	orderExpiry = 48 * time.Hour
	// An order that never got a gateway reference died while starting.
	startGrace = 10 * time.Minute
)

// settleMu serialises settlement of online orders so a double redirect and the
// reconciler never verify and deliver the same payment twice.
var settleMu sync.Mutex

// IsOnlineGateway reports whether the method is paid at a gateway's site.
func IsOnlineGateway(gateway string) bool { return isOnline(gateway) }

func isOnline(gateway string) bool {
	return isVerifiable(gateway) || gateway == GatewayCrypto
}

// isVerifiable marks the gateways whose payments the shop can confirm by
// asking them; crypto payments are reported by a signed webhook instead.
func isVerifiable(gateway string) bool {
	return gateway == GatewayZarinpal || gateway == GatewayIDPay
}

// gatewayAcceptsCurrency: the Iranian gateways take toman and rials, the
// crypto invoice is priced in dollars or euros.
func gatewayAcceptsCurrency(gateway, currency string) bool {
	switch gateway {
	case GatewayZarinpal, GatewayIDPay:
		_, ok := payment.RialAmount(0, currency)
		return ok
	case GatewayCrypto:
		return currency == "USD" || currency == "EUR"
	default:
		return true
	}
}

// SetGatewayFactory replaces the real payment clients; tests of other packages
// use it to point the gateways at fake servers.
func (s *OrderService) SetGatewayFactory(factory func(name string, cfg *SalesConfig) payment.Starter) {
	s.gatewayFactory = factory
}

// ShopOrderURL is where a customer lands after the gateway: the shop page
// showing the order.
func ShopOrderURL(cfg *SalesConfig, token string) string {
	return cfg.PublicBaseURL + "/shop/?order=" + url.QueryEscape(token)
}

// newGateway builds the client for an online method from the configuration.
func (s *OrderService) newGateway(name string, cfg *SalesConfig) payment.Starter {
	if s.gatewayFactory != nil {
		return s.gatewayFactory(name, cfg)
	}
	switch name {
	case GatewayZarinpal:
		return &payment.ZarinpalGateway{MerchantID: cfg.Zarinpal.MerchantId, Sandbox: cfg.Zarinpal.Sandbox}
	case GatewayIDPay:
		return &payment.IDPayGateway{APIKey: cfg.IDPay.ApiKey, Sandbox: cfg.IDPay.Sandbox}
	case GatewayCrypto:
		return &payment.NOWPaymentsGateway{APIKey: cfg.Crypto.ApiKey, Sandbox: cfg.Crypto.Sandbox}
	default:
		return nil
	}
}

func callbackURL(cfg *SalesConfig, order *model.Order) string {
	if order.Gateway == GatewayCrypto {
		return cfg.PublicBaseURL + "/shop/api/ipn/nowpayments"
	}
	return cfg.PublicBaseURL + "/shop/api/callback/" + order.Gateway + "?order=" + url.QueryEscape(order.Token)
}

// startRequest prices the order the way the gateway wants it: rials for the
// Iranian gateways, the plan currency for the crypto invoice.
func startRequest(cfg *SalesConfig, order *model.Order) (payment.StartRequest, bool) {
	req := payment.StartRequest{
		OrderToken:  order.Token,
		Description: order.PlanName,
		CallbackURL: callbackURL(cfg, order),
		ReturnURL:   ShopOrderURL(cfg, order.Token),
	}
	if order.Gateway == GatewayCrypto {
		req.Amount, req.Currency = order.Amount, order.Currency
		req.Description = "Order " + strconv.Itoa(order.Id)
		return req, true
	}
	rials, ok := payment.RialAmount(order.Amount, order.Currency)
	req.Amount, req.Currency = rials, "IRR"
	return req, ok
}

// StartPayment opens the order's payment at its gateway and stores where to
// send the customer. A gateway that refuses closes the order, freeing the
// username for another try.
func (s *OrderService) StartPayment(order *model.Order) (*model.Order, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	req, ok := startRequest(cfg, order)
	gw := s.newGateway(order.Gateway, cfg)
	if !ok || gw == nil {
		return nil, salesErr(ErrGatewayUnavailable)
	}
	res, err := gw.Start(context.Background(), req)
	if err != nil {
		logger.Warningf("order %d: starting %s payment: %v", order.Id, order.Gateway, err)
		if _, cerr := s.closeOrder(order.Id, model.OrderStatusFailed); cerr != nil {
			return nil, cerr
		}
		return nil, salesErr(ErrGatewayFailed)
	}
	err = database.GetDB().Model(&model.Order{}).Where("id = ?", order.Id).
		Updates(map[string]any{"gateway_ref": res.Ref, "pay_url": res.PayURL}).Error
	if err != nil {
		return nil, err
	}
	return s.getOrder(order.Id)
}

// closeOrder moves a pending order to a final status; any other status is kept.
func (s *OrderService) closeOrder(id int, status string) (*model.Order, error) {
	err := database.GetDB().Model(&model.Order{}).
		Where("id = ? AND status = ?", id, model.OrderStatusPending).
		Update("status", status).Error
	if err != nil {
		return nil, err
	}
	return s.getOrder(id)
}

// CompleteGatewayPayment settles an online order after its customer came back
// from the gateway. callbackPaid is only the redirect's claim: a payment is
// accepted solely when the gateway confirms it server to server. A gateway
// outage returns an error and leaves the order pending.
func (s *OrderService) CompleteGatewayPayment(inboundSvc *InboundService, token string, callbackPaid bool) (*model.Order, bool, error) {
	settleMu.Lock()
	defer settleMu.Unlock()
	order, err := s.GetOrderByToken(token)
	if err != nil {
		return nil, false, err
	}
	if !isVerifiable(order.Gateway) {
		return nil, false, salesErr(ErrOrderState)
	}
	switch order.Status {
	case model.OrderStatusPending:
	case model.OrderStatusPaid:
		return s.redeliver(inboundSvc, order)
	default:
		return order, false, nil
	}
	if !callbackPaid {
		closed, err := s.closeOrder(order.Id, model.OrderStatusFailed)
		return closed, false, err
	}
	res, err := s.verifyOnline(order)
	if err != nil {
		return order, false, err
	}
	return s.finishVerified(inboundSvc, order, res)
}

// redeliver retries the delivery of a paid order whose client is missing.
func (s *OrderService) redeliver(inboundSvc *InboundService, order *model.Order) (*model.Order, bool, error) {
	if order.FulfilledAt != 0 {
		return order, false, nil
	}
	needRestart, err := s.FulfilOrder(inboundSvc, order.Id)
	reloaded, rerr := s.getOrder(order.Id)
	if err == nil {
		err = rerr
	}
	return reloaded, needRestart, err
}

func (s *OrderService) verifyOnline(order *model.Order) (*payment.VerifyResult, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	amount, ok := payment.RialAmount(order.Amount, order.Currency)
	gw, verifiable := s.newGateway(order.Gateway, cfg).(payment.Gateway)
	if !ok || !verifiable || order.GatewayRef == "" {
		return nil, salesErr(ErrOrderState)
	}
	return gw.Verify(context.Background(), payment.VerifyRequest{Ref: order.GatewayRef, OrderToken: order.Token, Amount: amount})
}

func (s *OrderService) finishVerified(inboundSvc *InboundService, order *model.Order, res *payment.VerifyResult) (*model.Order, bool, error) {
	if !res.Paid {
		closed, err := s.closeOrder(order.Id, model.OrderStatusFailed)
		return closed, false, err
	}
	if err := database.GetDB().Model(&model.Order{}).Where("id = ?", order.Id).Update("receipt_ref", res.TrackingRef).Error; err != nil {
		return nil, false, err
	}
	needRestart, err := s.FulfilOrder(inboundSvc, order.Id)
	reloaded, rerr := s.getOrder(order.Id)
	if err == nil {
		err = rerr
	}
	return reloaded, needRestart, err
}

// ReconcileOnline settles pending online orders whose customer never came
// back (closed the tab after paying), and abandons the ones nobody paid.
func (s *OrderService) ReconcileOnline(inboundSvc *InboundService) (int, bool, error) {
	now := time.Now()
	var pending []model.Order
	err := database.GetDB().
		Where("gateway IN ? AND status = ? AND created_at <= ?", []string{GatewayZarinpal, GatewayIDPay}, model.OrderStatusPending, now.Add(-reconcileMinAge).UnixMilli()).
		Find(&pending).Error
	if err != nil {
		return 0, false, err
	}
	if err := s.expireCryptoOrders(now); err != nil {
		logger.Warning("shop reconcile: expiring crypto orders:", err)
	}
	fulfilled, needRestart := 0, false
	for i := range pending {
		done, restart := s.reconcileOne(inboundSvc, &pending[i], now)
		if done {
			fulfilled++
		}
		needRestart = needRestart || restart
	}
	return fulfilled, needRestart, nil
}

func (s *OrderService) reconcileOne(inboundSvc *InboundService, order *model.Order, now time.Time) (bool, bool) {
	settleMu.Lock()
	defer settleMu.Unlock()
	current, err := s.getOrder(order.Id)
	if err != nil || current.Status != model.OrderStatusPending {
		return false, false
	}
	created := time.UnixMilli(current.CreatedAt)
	if current.GatewayRef == "" {
		if now.Sub(created) > startGrace {
			_, _ = s.closeOrder(current.Id, model.OrderStatusFailed)
		}
		return false, false
	}
	res, err := s.verifyOnline(current)
	if err != nil {
		if !errors.Is(err, payment.ErrGateway) {
			logger.Warningf("order %d: reconcile: %v", current.Id, err)
		}
		return false, false
	}
	if !res.Paid {
		if now.Sub(created) > orderExpiry {
			_, _ = s.closeOrder(current.Id, model.OrderStatusCanceled)
		}
		return false, false
	}
	_, restart, err := s.finishVerified(inboundSvc, current, res)
	if err != nil {
		logger.Warningf("order %d: reconcile delivery: %v", current.Id, err)
		return false, restart
	}
	return true, restart
}
