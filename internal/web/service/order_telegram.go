package service

import (
	"errors"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const maxTelegramUsernames = 50

// checkTelegramOwner keeps a bot buyer to the clients bound to their own
// Telegram id: the binding stands in for the subscription id a visitor proves.
func (s *OrderService) checkTelegramOwner(in CreateOrderInput, email string) error {
	if in.TgId == 0 || in.Kind != model.OrderKindRenew {
		return nil
	}
	rec, err := s.clients.GetRecordByEmail(nil, email)
	if err != nil || rec.TgID != in.TgId {
		return salesErr(ErrEmailUnknown)
	}
	return nil
}

// TelegramUsername is the first free username of the form tg<id>, tg<id>-2...
// for a bot buyer, who is never asked to type one.
func (s *OrderService) TelegramUsername(tgId int64) (string, error) {
	base := "tg" + strconv.FormatInt(tgId, 10)
	for n := 1; n <= maxTelegramUsernames; n++ {
		name := base
		if n > 1 {
			name += "-" + strconv.Itoa(n)
		}
		if err := s.checkUsername(model.OrderKindNew, name); err == nil {
			return name, nil
		} else if !isSalesError(err, ErrEmailTaken) {
			return "", err
		}
	}
	return "", salesErr(ErrEmailTaken)
}

// TelegramOrders lists a bot buyer's own orders, newest first.
func (s *OrderService) TelegramOrders(tgId int64, limit int) ([]model.Order, error) {
	orders := []model.Order{}
	if tgId == 0 {
		return orders, nil
	}
	err := database.GetDB().Where("tg_id = ?", tgId).Order("id desc").Limit(limit).Find(&orders).Error
	return orders, err
}

// TelegramOrder returns one of the buyer's own orders; anyone else's is
// reported as not found.
func (s *OrderService) TelegramOrder(tgId int64, id int) (*model.Order, error) {
	order, err := s.getOrder(id)
	if err != nil {
		return nil, err
	}
	if tgId == 0 || order.TgId != tgId {
		return nil, salesErr(ErrOrderNotFound)
	}
	return order, nil
}

func isSalesError(err error, code string) bool { return SalesErrorCode(err) == code }

// SalesErrorCode is the customer-safe code of err, or "" when err is not a
// SalesError.
func SalesErrorCode(err error) string {
	var se *SalesError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}
