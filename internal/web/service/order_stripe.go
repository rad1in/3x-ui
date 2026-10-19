package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// HandleStripeWebhook settles an order from a Stripe Checkout event. The
// signature only authenticates the sender; the event is just a nudge, and the
// order is delivered only after the Checkout Session is read back from Stripe
// and matches the order's amount. A replayed event finds the order delivered
// and does nothing. A gateway outage returns an error so Stripe redelivers.
func (s *OrderService) HandleStripeWebhook(inboundSvc *InboundService, body []byte, signature string) (*model.Order, bool, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, false, err
	}
	if !gatewayEnabled(cfg, GatewayStripe) {
		return nil, false, salesErr(ErrGatewayUnavailable)
	}
	if !payment.VerifyStripeSignature(cfg.Stripe.WebhookSecret, body, signature, time.Now()) {
		return nil, false, salesErr(ErrBadSignature)
	}
	ev, err := payment.ParseStripeEvent(body)
	if err != nil {
		return nil, false, err
	}
	if ev.Type != payment.StripeSessionCompleted && ev.Type != payment.StripeAsyncPaymentSucceed {
		return nil, false, nil
	}

	settleMu.Lock()
	defer settleMu.Unlock()
	order, err := s.GetOrderByToken(ev.OrderToken)
	if err != nil {
		return nil, false, err
	}
	if order.Gateway != GatewayStripe || order.GatewayRef != ev.SessionID {
		return nil, false, salesErr(ErrOrderState)
	}
	switch order.Status {
	case model.OrderStatusPaid:
		return s.redeliver(inboundSvc, order)
	case model.OrderStatusPending:
		res, err := s.verifyOnline(order)
		if err != nil {
			return order, false, err
		}
		return s.finishVerified(inboundSvc, order, res)
	case model.OrderStatusFailed, model.OrderStatusCanceled:
		return s.lateOnlinePayment(order)
	default:
		return order, false, nil
	}
}

// lateOnlinePayment checks an order the shop already gave up on, such as one
// whose customer pressed back before paying; money that did arrive is parked
// for the admin instead of being lost.
func (s *OrderService) lateOnlinePayment(order *model.Order) (*model.Order, bool, error) {
	res, err := s.verifyOnline(order)
	if err != nil {
		return order, false, err
	}
	if !res.Paid && !res.Mismatch {
		return order, false, nil
	}
	ref := res.TrackingRef
	if ref == "" {
		ref = order.GatewayRef
	}
	return s.parkForReview(order, order.Status, ref, "late payment after the order was "+order.Status)
}
