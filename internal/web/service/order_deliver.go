package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// deliver creates the customer's client, or renews theirs, with the terms the
// order was bought under. Everything goes through ClientService so Xray and
// remote nodes are updated by the normal runtime path.
func (s *OrderService) deliver(inboundSvc *InboundService, order *model.Order) (bool, error) {
	if order.Kind == model.OrderKindRenew {
		return s.renew(inboundSvc, order)
	}
	tpl, err := s.clients.GetClientTemplate(order.PlanTemplate)
	if err != nil {
		return false, err
	}
	sold := *tpl
	sold.TotalGB, sold.ExpiryDays, sold.LimitIP = order.TotalGB, order.DurationDays, order.LimitIP
	return s.clients.Create(inboundSvc, &ClientCreatePayload{
		Client:     templateClient(&sold, order.ClientEmail, time.Now().UnixMilli()),
		InboundIds: append([]int(nil), tpl.InboundIds...),
	})
}

// renew restarts the client's period: usage is zeroed, the plan's quota and
// IP limit replace the old ones and the days are added after whatever time the
// client still has.
func (s *OrderService) renew(inboundSvc *InboundService, order *model.Order) (bool, error) {
	rec, err := s.clients.GetRecordByEmail(nil, order.ClientEmail)
	if err != nil {
		return false, err
	}
	needRestart, err := s.clients.ResetTrafficByEmail(inboundSvc, order.ClientEmail)
	if err != nil {
		return needRestart, err
	}
	now := time.Now().UnixMilli()
	updated := rec.ToClient()
	updated.Enable = true
	updated.TotalGB = order.TotalGB
	updated.LimitIP = order.LimitIP
	updated.ExpiryTime = renewedExpiry(rec.ExpiryTime, order.DurationDays, now)
	restart, err := s.clients.Update(inboundSvc, rec.Id, *updated, rec.LimitHwid)
	return needRestart || restart, err
}

// renewedExpiry stacks days on the unused remainder; a lapsed or not yet
// started client begins at now, and 0 days means no expiry.
func renewedExpiry(current int64, days int, now int64) int64 {
	if days == 0 {
		return 0
	}
	base := now
	if current > now {
		base = current
	}
	return base + int64(days)*millisPerDay
}
