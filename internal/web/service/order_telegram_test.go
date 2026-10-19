package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func bindTg(t *testing.T, email string, tgID int64) {
	t.Helper()
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", email).Update("tg_id", tgID).Error; err != nil {
		t.Fatalf("bind %s: %v", email, err)
	}
}

func TestTelegramNewPurchaseLinksTheBuyerToTheNewClient(t *testing.T) {
	f := newShopFixture(t, 29101)
	name, err := f.orders.TelegramUsername(555)
	if err != nil || name != "tg555" {
		t.Fatalf("TelegramUsername = %q, %v; want tg555", name, err)
	}
	o, _, err := f.orders.Checkout(&InboundService{}, CreateOrderInput{
		PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: name, Gateway: GatewayCard, TgId: 555,
	})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	recs, err := (&ClientService{}).GetRecordsByTgID(555)
	if err != nil || len(recs) != 1 || recs[0].Email != "tg555" {
		t.Fatalf("clients of tg 555 = %v, %v; want the delivered tg555", recs, err)
	}
	next, err := f.orders.TelegramUsername(555)
	if err != nil || next != "tg555-2" {
		t.Fatalf("second TelegramUsername = %q, %v; want tg555-2 once tg555 exists", next, err)
	}
}

func TestTelegramRenewalOnlyReachesTheBuyersOwnClient(t *testing.T) {
	f := shopWithWallet(t, 29102)
	seedClient(t, f, "mine")
	seedClient(t, f, "theirs")
	bindTg(t, "mine", 555)
	bindTg(t, "theirs", 666)
	if _, err := (&WalletService{}).Adjust("mine", "IRT", 300000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if _, err := (&WalletService{}).Adjust("theirs", "IRT", 300000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}

	_, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "theirs", Gateway: GatewayWallet, TgId: 555})
	wantSalesError(t, err, ErrEmailUnknown)
	if got := balanceOf(t, "theirs", "IRT"); got != 300000 {
		t.Fatalf("foreign wallet = %d, want untouched 300000", got)
	}

	o, _, err := f.orders.Checkout(&InboundService{}, CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "mine", Gateway: GatewayWallet, TgId: 555})
	if err != nil || o.Status != model.OrderStatusPaid || o.FulfilledAt == 0 {
		t.Fatalf("own wallet renewal = %+v, %v; want paid and delivered with no subscription id", o, err)
	}
	if got := balanceOf(t, "mine", "IRT"); got != 50000 {
		t.Fatalf("balance = %d, want 300000 - 250000", got)
	}
}

func TestTelegramOrdersAreVisibleOnlyToTheirBuyer(t *testing.T) {
	f := newShopFixture(t, 29103)
	mine, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "tg555", Gateway: GatewayCard, TgId: 555})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "tg666", Gateway: GatewayCard, TgId: 666}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	f.newOrder(t, "website-buyer")

	list, err := f.orders.TelegramOrders(555, 10)
	if err != nil || len(list) != 1 || list[0].Id != mine.Id {
		t.Fatalf("TelegramOrders(555) = %v, %v; want only order %d", list, err, mine.Id)
	}
	if _, err := f.orders.TelegramOrder(666, mine.Id); err == nil {
		t.Fatal("tg 666 read tg 555's order")
	} else {
		wantSalesError(t, err, ErrOrderNotFound)
	}
	if _, err := f.orders.TelegramOrder(555, mine.Id); err != nil {
		t.Fatalf("owner TelegramOrder: %v", err)
	}
}
