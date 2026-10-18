package service

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const maxOrderPage = 200

// OrderPage is one page of orders, newest first, with the unpaged total.
type OrderPage struct {
	Items []model.Order `json:"items"`
	Total int64         `json:"total"`
}

// OrderService runs the purchase lifecycle of plans.
type OrderService struct{}

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

// randomOrderToken is the unguessable handle a customer uses to follow an order.
func randomOrderToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
