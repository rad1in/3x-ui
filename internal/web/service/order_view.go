package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// ShopPlan is a plan as the public shop shows it.
type ShopPlan struct {
	Id           int    `json:"id" example:"1"`
	Name         string `json:"name" example:"Monthly 50 GB"`
	Description  string `json:"description"`
	Price        int64  `json:"price" example:"250000"`
	Currency     string `json:"currency" example:"IRT"`
	TotalGB      int64  `json:"totalGB" example:"53687091200"`
	DurationDays int    `json:"durationDays" example:"30"`
	LimitIP      int    `json:"limitIp" example:"2"`
}

// ShopCatalog is what the public shop lists: sellable plans and the payment
// methods that are switched on.
type ShopCatalog struct {
	Plans   []ShopPlan `json:"plans"`
	Methods []string   `json:"methods"`
}

// ShopOrderView is an order as its customer sees it: no internal ids, and the
// card details only while a transfer is still expected.
type ShopOrderView struct {
	Token           string         `json:"token" example:"3f9c2a7e5b1d4c8a9e0f6b2d7a1c4e8f"`
	PlanName        string         `json:"planName" example:"Monthly 50 GB"`
	Kind            string         `json:"kind" example:"new"`
	ClientEmail     string         `json:"clientEmail" example:"alice"`
	Amount          int64          `json:"amount" example:"250000"`
	Currency        string         `json:"currency" example:"IRT"`
	Gateway         string         `json:"gateway" example:"card"`
	Status          string         `json:"status" example:"pending"`
	RejectReason    string         `json:"rejectReason"`
	CreatedAt       int64          `json:"createdAt"`
	Fulfilled       bool           `json:"fulfilled"`
	SubscriptionURL string         `json:"subscriptionUrl"`
	Card            *CardPayConfig `json:"card,omitempty"`
	PayURL          string         `json:"payUrl,omitempty"`
}

func enabledMethods(cfg *SalesConfig) []string {
	methods := []string{}
	for _, gateway := range []string{GatewayCard, GatewayZarinpal, GatewayIDPay} {
		if gatewayEnabled(cfg, gateway) {
			methods = append(methods, gateway)
		}
	}
	return methods
}

// Catalog lists what the public shop can sell right now.
func (s *OrderService) Catalog() (*ShopCatalog, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	plans, err := s.plans.ListPlans(true)
	if err != nil {
		return nil, err
	}
	catalog := &ShopCatalog{Plans: make([]ShopPlan, 0, len(plans)), Methods: enabledMethods(cfg)}
	for _, p := range plans {
		catalog.Plans = append(catalog.Plans, ShopPlan{
			Id: p.Id, Name: p.Name, Description: p.Description, Price: p.Price, Currency: p.Currency,
			TotalGB: p.TotalGB, DurationDays: p.DurationDays, LimitIP: p.LimitIP,
		})
	}
	return catalog, nil
}

// PublicView renders order for its customer; host names the machine the
// subscription server is reached on.
func (s *OrderService) PublicView(order *model.Order, host string) (*ShopOrderView, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	view := &ShopOrderView{
		Token: order.Token, PlanName: order.PlanName, Kind: order.Kind, ClientEmail: order.ClientEmail,
		Amount: order.Amount, Currency: order.Currency, Gateway: order.Gateway, Status: order.Status,
		RejectReason: order.RejectReason, CreatedAt: order.CreatedAt, Fulfilled: order.FulfilledAt > 0,
	}
	awaitingTransfer := order.Status == model.OrderStatusPending || order.Status == model.OrderStatusReview
	if order.Gateway == GatewayCard && awaitingTransfer && cfg.Card.Enabled {
		card := cfg.Card
		view.Card = &card
	}
	if isOnline(order.Gateway) && order.Status == model.OrderStatusPending {
		view.PayURL = order.PayURL
	}
	if view.Fulfilled {
		view.SubscriptionURL = s.subscriptionURL(order.ClientEmail, host)
	}
	return view, nil
}

// subscriptionURL is empty when the subscription server is off or the client
// has since been removed.
func (s *OrderService) subscriptionURL(email, host string) string {
	var settings SettingService
	if enabled, err := settings.GetSubEnable(); err != nil || !enabled {
		return ""
	}
	rec, err := s.clients.GetRecordByEmail(nil, email)
	if err != nil || rec.SubID == "" {
		return ""
	}
	path, err := settings.GetSubPath()
	if err != nil {
		return ""
	}
	return strings.TrimRight(settings.BuildSubURIBase(host), "/") + "/" + strings.Trim(path, "/") + "/" + rec.SubID
}
