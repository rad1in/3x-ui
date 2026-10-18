package service

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type shopFixture struct {
	orders *OrderService
	plan   *model.Plan
	tpl    *model.ClientTemplate
}

func newShopFixture(t *testing.T, port int) *shopFixture {
	t.Helper()
	setupBulkDB(t)
	ib := mkInbound(t, port, model.VLESS, `{"clients":[]}`)
	tpl, err := (&ClientService{}).SaveClientTemplate(&model.ClientTemplate{
		Name: "shop-tpl", Group: "shop", InboundIds: []int{ib.Id},
	})
	if err != nil {
		t.Fatalf("SaveClientTemplate: %v", err)
	}
	plan, err := (&PlanService{}).SavePlan(&model.Plan{
		Name: "Monthly", Price: 250000, Currency: "IRT", TotalGB: 50 << 30, DurationDays: 30, LimitIP: 2,
		TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	cfg := &SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: "6037991812345678"}}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	return &shopFixture{orders: &OrderService{}, plan: plan, tpl: tpl}
}

func (f *shopFixture) newOrder(t *testing.T, email string) *model.Order {
	t.Helper()
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: email, Gateway: GatewayCard})
	if err != nil {
		t.Fatalf("CreateOrder(%s): %v", email, err)
	}
	return o
}

func wantSalesError(t *testing.T, err error, code string) {
	t.Helper()
	var se *SalesError
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("error = %v, want sales error %q", err, code)
	}
}

func TestCreateOrderRefusals(t *testing.T) {
	f := newShopFixture(t, 25001)
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "taken", Enable: true}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	f.newOrder(t, "open-one")

	hidden := *f.plan
	hidden.Id, hidden.Name, hidden.Enabled = 0, "Hidden", false
	hiddenPlan, err := (&PlanService{}).SavePlan(&hidden)
	if err != nil {
		t.Fatalf("SavePlan hidden: %v", err)
	}

	cases := []struct {
		name string
		in   CreateOrderInput
		code string
	}{
		{"disabled plan", CreateOrderInput{PlanId: hiddenPlan.Id, Kind: model.OrderKindNew, Email: "fresh", Gateway: GatewayCard}, ErrPlanUnavailable},
		{"unknown plan", CreateOrderInput{PlanId: 999, Kind: model.OrderKindNew, Email: "fresh", Gateway: GatewayCard}, ErrPlanUnavailable},
		{"bad username", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "a b!", Gateway: GatewayCard}, ErrInvalidEmail},
		{"too short username", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "ab", Gateway: GatewayCard}, ErrInvalidEmail},
		{"existing client", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "taken", Gateway: GatewayCard}, ErrEmailTaken},
		{"username with an open order", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "open-one", Gateway: GatewayCard}, ErrEmailTaken},
		{"renew unknown client", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "ghost", Gateway: GatewayCard}, ErrEmailUnknown},
		{"unknown gateway", CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "fresh", Gateway: "bitcoin"}, ErrGatewayUnavailable},
		{"unknown kind", CreateOrderInput{PlanId: f.plan.Id, Kind: "gift", Email: "fresh", Gateway: GatewayCard}, ErrInvalidKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.orders.CreateOrder(tc.in)
			wantSalesError(t, err, tc.code)
		})
	}
}

func TestCreateOrderRefusesCardWhenCardIsOff(t *testing.T) {
	f := newShopFixture(t, 25002)
	if err := (&SalesConfigService{}).Save(&SalesConfig{}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	_, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayCard})
	wantSalesError(t, err, ErrGatewayUnavailable)
}

func TestOrderKeepsThePlanTermsItWasBoughtUnder(t *testing.T) {
	f := newShopFixture(t, 25003)
	o := f.newOrder(t, "alice")

	edited := *f.plan
	edited.TotalGB, edited.DurationDays, edited.Price = 1<<30, 1, 1
	if _, err := (&PlanService{}).SavePlan(&edited); err != nil {
		t.Fatalf("edit plan: %v", err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	rec := lookupClientRecord(t, "alice")
	if rec.TotalGB != 50<<30 || rec.LimitIP != 2 {
		t.Fatalf("client = quota %d, ip %d; want the 50 GB / 2 IP the customer bought", rec.TotalGB, rec.LimitIP)
	}
}

func TestSubmitReceiptMovesOrderToReview(t *testing.T) {
	f := newShopFixture(t, 25004)
	o := f.newOrder(t, "alice")

	_, err := f.orders.SubmitReceipt(o.Token, "  ", "")
	wantSalesError(t, err, ErrReceiptInvalid)
	_, err = f.orders.SubmitReceipt("nope", "123456", "")
	wantSalesError(t, err, ErrOrderNotFound)

	got, err := f.orders.SubmitReceipt(o.Token, " 482913 ", "paid at 14:20")
	if err != nil {
		t.Fatalf("SubmitReceipt: %v", err)
	}
	if got.Status != model.OrderStatusReview || got.ReceiptRef != "482913" || got.ReceiptNote != "paid at 14:20" {
		t.Fatalf("order = %+v, want review with the trimmed receipt", got)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	_, err = f.orders.SubmitReceipt(o.Token, "999999", "")
	wantSalesError(t, err, ErrOrderState)
}

func TestFulfilNewOrderCreatesTheClientOnce(t *testing.T) {
	f := newShopFixture(t, 25005)
	o := f.newOrder(t, "alice")
	if _, err := f.orders.SubmitReceipt(o.Token, "111111", ""); err != nil {
		t.Fatalf("SubmitReceipt: %v", err)
	}

	before := time.Now().UnixMilli()
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	rec := lookupClientRecord(t, "alice")
	wantExpiry := before + 30*24*3600*1000
	if rec.TotalGB != 50<<30 || rec.LimitIP != 2 || rec.Group != "shop" || !rec.Enable {
		t.Fatalf("client = %+v, want plan quota/limit and template group", rec)
	}
	if rec.ExpiryTime < wantExpiry || rec.ExpiryTime > wantExpiry+60*1000 {
		t.Fatalf("expiry = %d, want about now+30d (%d)", rec.ExpiryTime, wantExpiry)
	}
	var after model.Order
	if err := database.GetDB().First(&after, o.Id).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if after.Status != model.OrderStatusPaid || after.PaidAt == 0 || after.FulfilledAt == 0 {
		t.Fatalf("order = %+v, want paid and fulfilled", after)
	}

	_, err := f.orders.FulfilOrder(&InboundService{}, o.Id)
	wantSalesError(t, err, ErrOrderState)
}

func TestFulfilRunsOnceWhenApprovedConcurrently(t *testing.T) {
	f := newShopFixture(t, 25006)
	future := time.Now().UnixMilli() + 10*24*3600*1000
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true, ExpiryTime: future}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayCard})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.orders.FulfilOrder(&InboundService{}, o.Id)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d approvals succeeded (%v), want exactly one", ok, errs)
	}
	if got, want := lookupClientRecord(t, "alice").ExpiryTime, future+30*24*3600*1000; got != want {
		t.Fatalf("expiry = %d, want %d: the 30 days were added more than once", got, want)
	}
}

func TestFulfilRenewExtendsFromLaterOfNowAndExpiryAndResetsTraffic(t *testing.T) {
	f := newShopFixture(t, 25007)
	now := time.Now().UnixMilli()
	future := now + 10*24*3600*1000
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client:     model.Client{Email: "bob", Enable: false, TotalGB: 5 << 30, ExpiryTime: future},
		InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "bob").
		Updates(map[string]any{"up": 3 << 30, "down": 2 << 30, "enable": false}).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "bob", Gateway: GatewayCard})
	if err != nil {
		t.Fatalf("CreateOrder renew: %v", err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}

	rec := lookupClientRecord(t, "bob")
	if want := future + 30*24*3600*1000; rec.ExpiryTime != want {
		t.Fatalf("expiry = %d, want the 10 remaining days kept: %d", rec.ExpiryTime, want)
	}
	if rec.TotalGB != 50<<30 || rec.LimitIP != 2 || !rec.Enable {
		t.Fatalf("client = %+v, want the plan quota, limit and enabled", rec)
	}
	var tr xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "bob").First(&tr).Error; err != nil {
		t.Fatalf("traffic row: %v", err)
	}
	if tr.Up != 0 || tr.Down != 0 || !tr.Enable {
		t.Fatalf("traffic = %+v, want zeroed usage and enabled", tr)
	}
}

func TestFulfilRenewOfAnExpiredClientStartsFromNow(t *testing.T) {
	f := newShopFixture(t, 25008)
	past := time.Now().UnixMilli() - 40*24*3600*1000
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "carl", Enable: true, ExpiryTime: past}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "carl", Gateway: GatewayCard})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	start := time.Now().UnixMilli()
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	got := lookupClientRecord(t, "carl").ExpiryTime
	if want := start + 30*24*3600*1000; got < want || got > want+60*1000 {
		t.Fatalf("expiry = %d, want about now+30d (%d), not the lapsed date plus 30d", got, want)
	}
}

func TestRejectOrderIsFinal(t *testing.T) {
	f := newShopFixture(t, 25009)
	o := f.newOrder(t, "alice")
	if _, err := f.orders.SubmitReceipt(o.Token, "111111", ""); err != nil {
		t.Fatalf("SubmitReceipt: %v", err)
	}
	got, err := f.orders.RejectOrder(o.Id, "  amount does not match ")
	if err != nil {
		t.Fatalf("RejectOrder: %v", err)
	}
	if got.Status != model.OrderStatusRejected || got.RejectReason != "amount does not match" {
		t.Fatalf("order = %+v, want rejected with the trimmed reason", got)
	}
	_, err = f.orders.FulfilOrder(&InboundService{}, o.Id)
	wantSalesError(t, err, ErrOrderState)
	var count int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Count(&count)
	if count != 0 {
		t.Fatalf("a rejected order created %d clients", count)
	}
}

func TestFailedFulfilmentStaysPaidAndCanBeRetried(t *testing.T) {
	f := newShopFixture(t, 25010)
	o := f.newOrder(t, "alice")
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed colliding client: %v", err)
	}

	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err == nil {
		t.Fatal("FulfilOrder succeeded although the username was taken meanwhile")
	}
	var stuck model.Order
	if err := database.GetDB().First(&stuck, o.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stuck.Status != model.OrderStatusPaid || stuck.FulfilledAt != 0 {
		t.Fatalf("order = %+v, want paid and still unfulfilled so it can be retried", stuck)
	}

	if _, err := (&ClientService{}).DeleteByEmail(&InboundService{}, "alice", false); err != nil {
		t.Fatalf("remove collision: %v", err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if rec := lookupClientRecord(t, "alice"); rec.TotalGB != 50<<30 {
		t.Fatalf("client after retry = %+v", rec)
	}
}

func TestCatalogListsOnlyEnabledPlansAndMethods(t *testing.T) {
	f := newShopFixture(t, 25011)
	hidden := *f.plan
	hidden.Id, hidden.Name, hidden.Enabled = 0, "Hidden", false
	if _, err := (&PlanService{}).SavePlan(&hidden); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	got, err := f.orders.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(got.Plans) != 1 || got.Plans[0].Name != "Monthly" || len(got.Methods) != 1 || got.Methods[0] != GatewayCard {
		t.Fatalf("catalog = %+v, want the one enabled plan and the card method", got)
	}
}

func TestPublicViewShowsCardOnlyWhileAwaitingTransferAndLinkOnceDelivered(t *testing.T) {
	f := newShopFixture(t, 25012)
	settings := SettingService{}
	if err := settings.setBool("subEnable", true); err != nil {
		t.Fatalf("enable sub: %v", err)
	}
	o := f.newOrder(t, "alice")

	view, err := f.orders.PublicView(o, "shop.example.com")
	if err != nil {
		t.Fatalf("PublicView: %v", err)
	}
	if view.Card == nil || view.Card.CardNumber != "6037991812345678" || view.SubscriptionURL != "" {
		t.Fatalf("pending view = %+v, want the card and no link yet", view)
	}

	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	paid, _ := f.orders.GetOrderByToken(o.Token)
	view, err = f.orders.PublicView(paid, "shop.example.com")
	if err != nil {
		t.Fatalf("PublicView paid: %v", err)
	}
	subID := lookupClientRecord(t, "alice").SubID
	if view.Card != nil || !view.Fulfilled || !strings.HasSuffix(view.SubscriptionURL, "/"+subID) || !strings.HasPrefix(view.SubscriptionURL, "http://shop.example.com") {
		t.Fatalf("paid view = %+v, want no card and a subscription link ending in the sub id", view)
	}
}
