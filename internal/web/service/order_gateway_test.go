package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// stubGateway is the boundary to the provider; the HTTP side is covered in
// the payment package against httptest servers.
type stubGateway struct {
	mu          sync.Mutex
	startReq    payment.StartRequest
	startErr    error
	verifyReq   payment.VerifyRequest
	verifyCalls int
	verifyRes   *payment.VerifyResult
	verifyErr   error
}

func (g *stubGateway) Start(_ context.Context, req payment.StartRequest) (*payment.StartResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.startReq = req
	if g.startErr != nil {
		return nil, g.startErr
	}
	return &payment.StartResult{Ref: "AUTH-1", PayURL: "https://gw.test/pay/AUTH-1"}, nil
}

func (g *stubGateway) Verify(_ context.Context, req payment.VerifyRequest) (*payment.VerifyResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.verifyCalls++
	g.verifyReq = req
	return g.verifyRes, g.verifyErr
}

func (g *stubGateway) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.verifyCalls
}

type onlineFixture struct {
	*shopFixture
	gw *stubGateway
}

func newOnlineFixture(t *testing.T, port int) *onlineFixture {
	t.Helper()
	f := newShopFixture(t, port)
	cfg := &SalesConfig{
		PublicBaseURL: "https://shop.example.com",
		Zarinpal:      ZarinpalConfig{Enabled: true, MerchantId: "merchant"},
	}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	gw := &stubGateway{verifyRes: &payment.VerifyResult{Paid: true, TrackingRef: "TRK-77"}}
	f.orders.gatewayFactory = func(string, *SalesConfig) payment.Starter { return gw }
	return &onlineFixture{shopFixture: f, gw: gw}
}

func (f *onlineFixture) startedOrder(t *testing.T, email string) *model.Order {
	t.Helper()
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: email, Gateway: GatewayZarinpal})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	o, err = f.orders.StartPayment(o)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	return o
}

func clientCount(t *testing.T, email string) int64 {
	t.Helper()
	var n int64
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", email).Count(&n).Error; err != nil {
		t.Fatalf("count clients: %v", err)
	}
	return n
}

func TestOnlineOrderNeedsAnEnabledMethodAndAnIranianCurrency(t *testing.T) {
	f := newOnlineFixture(t, 27001)
	usd := *f.plan
	usd.Id, usd.Name, usd.Currency, usd.Price = 0, "Dollar", "USD", 5
	usdPlan, err := (&PlanService{}).SavePlan(&usd)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	_, err = f.orders.CreateOrder(CreateOrderInput{PlanId: usdPlan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayZarinpal})
	wantSalesError(t, err, ErrCurrencyUnsupported)
	_, err = f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayIDPay})
	wantSalesError(t, err, ErrGatewayUnavailable)
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayZarinpal}); err != nil {
		t.Fatalf("CreateOrder zarinpal on an IRT plan: %v", err)
	}
}

func TestStartPaymentSendsRialsAndACallbackCarryingTheOrderToken(t *testing.T) {
	f := newOnlineFixture(t, 27002)
	o := f.startedOrder(t, "alice")

	if f.gw.startReq.Amount != 2500000 {
		t.Fatalf("gateway amount = %d, want 250000 toman as 2500000 rials", f.gw.startReq.Amount)
	}
	wantCallback := "https://shop.example.com/shop/api/callback/zarinpal?order=" + o.Token
	if f.gw.startReq.CallbackURL != wantCallback || f.gw.startReq.OrderToken != o.Token {
		t.Fatalf("start request = %+v, want callback %q", f.gw.startReq, wantCallback)
	}
	if o.GatewayRef != "AUTH-1" || o.PayURL != "https://gw.test/pay/AUTH-1" || o.Status != model.OrderStatusPending {
		t.Fatalf("order = %+v, want the gateway ref and pay url stored on a pending order", o)
	}
	view, err := f.orders.PublicView(o, "shop.example.com")
	if err != nil || view.PayURL != o.PayURL {
		t.Fatalf("view = %+v, %v; want the customer to get the pay url", view, err)
	}
}

func TestStartPaymentFailureClosesTheOrder(t *testing.T) {
	f := newOnlineFixture(t, 27003)
	f.gw.startErr = payment.ErrGateway
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayZarinpal})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	_, err = f.orders.StartPayment(o)
	wantSalesError(t, err, ErrGatewayFailed)
	after, _ := f.orders.GetOrder(o.Id)
	if after.Status != model.OrderStatusFailed {
		t.Fatalf("status = %q, want failed so the username is free again", after.Status)
	}
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayZarinpal}); err != nil {
		t.Fatalf("retry after a failed start: %v", err)
	}
}

func TestVerifiedCallbackDeliversTheClientOnce(t *testing.T) {
	f := newOnlineFixture(t, 27004)
	o := f.startedOrder(t, "alice")

	got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
	if err != nil {
		t.Fatalf("CompleteGatewayPayment: %v", err)
	}
	if got.Status != model.OrderStatusPaid || got.FulfilledAt == 0 || got.ReceiptRef != "TRK-77" {
		t.Fatalf("order = %+v, want paid, delivered and carrying the tracking code", got)
	}
	if f.gw.verifyReq.Ref != "AUTH-1" || f.gw.verifyReq.Amount != 2500000 || f.gw.verifyReq.OrderToken != o.Token {
		t.Fatalf("verify request = %+v, want the stored ref and the rial amount of the order", f.gw.verifyReq)
	}
	if rec := lookupClientRecord(t, "alice"); rec.TotalGB != 50<<30 {
		t.Fatalf("client = %+v", rec)
	}

	again, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
	if err != nil || again.Status != model.OrderStatusPaid {
		t.Fatalf("second callback = %+v, %v; want an idempotent success", again, err)
	}
	if f.gw.calls() != 1 || clientCount(t, "alice") != 1 {
		t.Fatalf("verify calls = %d, clients = %d; a repeated callback must not re-verify or re-deliver", f.gw.calls(), clientCount(t, "alice"))
	}
}

func TestCallbackNeverTrustsItsOwnClaimOfPayment(t *testing.T) {
	f := newOnlineFixture(t, 27005)
	f.gw.verifyRes = &payment.VerifyResult{Paid: false}
	o := f.startedOrder(t, "alice")

	got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
	if err != nil {
		t.Fatalf("CompleteGatewayPayment: %v", err)
	}
	if got.Status != model.OrderStatusFailed || clientCount(t, "alice") != 0 {
		t.Fatalf("order = %+v, clients = %d; an unverified payment must not deliver", got, clientCount(t, "alice"))
	}
}

func TestCanceledCallbackFailsTheOrderWithoutAskingTheGateway(t *testing.T) {
	f := newOnlineFixture(t, 27006)
	o := f.startedOrder(t, "alice")
	got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, false)
	if err != nil {
		t.Fatalf("CompleteGatewayPayment: %v", err)
	}
	if got.Status != model.OrderStatusFailed || f.gw.calls() != 0 {
		t.Fatalf("order = %+v, verify calls = %d; a cancelled payment is final", got, f.gw.calls())
	}
}

func TestGatewayOutageKeepsTheOrderPendingForARetry(t *testing.T) {
	f := newOnlineFixture(t, 27007)
	f.gw.verifyRes, f.gw.verifyErr = nil, payment.ErrGateway
	o := f.startedOrder(t, "alice")

	if _, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true); !errors.Is(err, payment.ErrGateway) {
		t.Fatalf("error = %v, want the gateway error", err)
	}
	if after, _ := f.orders.GetOrder(o.Id); after.Status != model.OrderStatusPending {
		t.Fatalf("status = %q, want pending so a later callback or the reconciler can settle it", after.Status)
	}

	f.gw.verifyRes, f.gw.verifyErr = &payment.VerifyResult{Paid: true, TrackingRef: "T"}, nil
	if got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true); err != nil || got.Status != model.OrderStatusPaid {
		t.Fatalf("retry = %+v, %v", got, err)
	}
}

func TestCallbackRefusesCardOrdersAndUnknownTokens(t *testing.T) {
	f := newOnlineFixture(t, 27008)
	if err := (&SalesConfigService{}).Save(&SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: "6037991812345678"}}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	card := f.newOrder(t, "carl")
	_, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, card.Token, true)
	wantSalesError(t, err, ErrOrderState)
	_, _, err = f.orders.CompleteGatewayPayment(&InboundService{}, "nope", true)
	wantSalesError(t, err, ErrOrderNotFound)
	if f.gw.calls() != 0 || clientCount(t, "carl") != 0 {
		t.Fatal("a card order must never be settled through the gateway callback")
	}
}

func TestConcurrentCallbacksRenewOnce(t *testing.T) {
	f := newOnlineFixture(t, 27009)
	future := time.Now().UnixMilli() + 10*24*3600*1000
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true, ExpiryTime: future}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayZarinpal})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := f.orders.StartPayment(o); err != nil {
		t.Fatalf("StartPayment: %v", err)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
		}()
	}
	wg.Wait()
	if got, want := lookupClientRecord(t, "alice").ExpiryTime, future+30*24*3600*1000; got != want {
		t.Fatalf("expiry = %d, want %d: concurrent callbacks renewed more than once", got, want)
	}
	if f.gw.calls() != 1 {
		t.Fatalf("gateway verified %d times, want once for concurrent callbacks of one order", f.gw.calls())
	}
}

func TestCallbackOnACardOrderLeavesItUntouched(t *testing.T) {
	f := newOnlineFixture(t, 27011)
	if err := (&SalesConfigService{}).Save(&SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: "6037991812345678"}}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	card := f.newOrder(t, "carl")
	_, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, card.Token, false)
	wantSalesError(t, err, ErrOrderState)
	if after, _ := f.orders.GetOrder(card.Id); after.Status != model.OrderStatusPending {
		t.Fatalf("status = %q, a stray cancelled callback must not close a card order", after.Status)
	}
}

func TestCallbackRetriesTheDeliveryOfAPaidOrder(t *testing.T) {
	f := newOnlineFixture(t, 27012)
	o := f.startedOrder(t, "alice")
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed colliding client: %v", err)
	}
	got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
	if err == nil || got.Status != model.OrderStatusPaid || got.FulfilledAt != 0 {
		t.Fatalf("first callback = %+v, %v; want a paid order whose delivery failed", got, err)
	}
	if _, err := (&ClientService{}).DeleteByEmail(&InboundService{}, "alice", false); err != nil {
		t.Fatalf("remove collision: %v", err)
	}
	got, _, err = f.orders.CompleteGatewayPayment(&InboundService{}, o.Token, true)
	if err != nil || got.FulfilledAt == 0 || clientCount(t, "alice") != 1 {
		t.Fatalf("second callback = %+v, %v; want the delivery completed without paying twice", got, err)
	}
	if f.gw.calls() != 1 {
		t.Fatalf("verify calls = %d, want 1: a paid order is not verified again", f.gw.calls())
	}
}

func TestReconcileSettlesPaidOrdersAndExpiresAbandonedOnes(t *testing.T) {
	f := newOnlineFixture(t, 27010)
	paid := f.startedOrder(t, "paid-one")
	young := f.startedOrder(t, "young-one")
	stale := f.startedOrder(t, "stale-one")
	db := database.GetDB()
	old := time.Now().Add(-10 * time.Minute).UnixMilli()
	for _, o := range []*model.Order{paid, stale} {
		if err := db.Model(&model.Order{}).Where("id = ?", o.Id).Update("created_at", old).Error; err != nil {
			t.Fatalf("age order: %v", err)
		}
	}
	ancient := time.Now().Add(-49 * time.Hour).UnixMilli()
	if err := db.Model(&model.Order{}).Where("id = ?", stale.Id).Update("created_at", ancient).Error; err != nil {
		t.Fatalf("age stale: %v", err)
	}
	if err := db.Model(&model.Order{}).Where("id = ?", paid.Id).Update("gateway_ref", "PAID-REF").Error; err != nil {
		t.Fatalf("mark: %v", err)
	}
	perRef := &refGateway{paidRefs: map[string]bool{"PAID-REF": true}}
	f.orders.gatewayFactory = func(string, *SalesConfig) payment.Starter { return perRef }

	fulfilled, _, err := f.orders.ReconcileOnline(&InboundService{})
	if err != nil {
		t.Fatalf("ReconcileOnline: %v", err)
	}
	if fulfilled != 1 || clientCount(t, "paid-one") != 1 {
		t.Fatalf("fulfilled = %d, paid-one clients = %d; want the paid order delivered", fulfilled, clientCount(t, "paid-one"))
	}
	status := func(o *model.Order) string { r, _ := f.orders.GetOrder(o.Id); return r.Status }
	if status(young) != model.OrderStatusPending || status(stale) != model.OrderStatusCanceled {
		t.Fatalf("young = %q, stale = %q; want the fresh order left alone and the 49 hour old one canceled", status(young), status(stale))
	}
	if slices.Contains(perRef.asked, young.Token) {
		t.Fatal("an order under two minutes old was verified; the customer may still be paying")
	}
}

// refGateway reports paid for chosen refs only.
type refGateway struct {
	mu       sync.Mutex
	paidRefs map[string]bool
	asked    []string
}

func (g *refGateway) Start(context.Context, payment.StartRequest) (*payment.StartResult, error) {
	return nil, errors.New("unused")
}

func (g *refGateway) Verify(_ context.Context, req payment.VerifyRequest) (*payment.VerifyResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asked = append(g.asked, req.OrderToken)
	if g.paidRefs[req.Ref] {
		return &payment.VerifyResult{Paid: true, TrackingRef: "R"}, nil
	}
	return &payment.VerifyResult{}, nil
}

func TestStripeOrderRefusesIranianCurrenciesAndKeepsAnUndecidedPaymentPending(t *testing.T) {
	f := newOnlineFixture(t, 27011)
	cfg := &SalesConfig{PublicBaseURL: "https://shop.example.com", Stripe: StripeConfig{Enabled: true, SecretKey: "sk_test"}}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	_, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayStripe})
	wantSalesError(t, err, ErrCurrencyUnsupported)

	usd := *f.plan
	usd.Id, usd.Name, usd.Currency, usd.Price = 0, "Dollar", "USD", 1999
	plan, err := (&PlanService{}).SavePlan(&usd)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	order, err := f.orders.CreateOrder(CreateOrderInput{PlanId: plan.Id, Kind: model.OrderKindNew, Email: "bob", Gateway: GatewayStripe})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order, err = f.orders.StartPayment(order); err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	if f.gw.startReq.Amount != 1999 || f.gw.startReq.Currency != "USD" || f.gw.startReq.CancelURL == "" {
		t.Fatalf("start request = %+v, want the plan price in its own currency and a cancel URL", f.gw.startReq)
	}

	f.gw.verifyRes = &payment.VerifyResult{Pending: true}
	got, _, err := f.orders.CompleteGatewayPayment(&InboundService{}, order.Token, true)
	if err != nil || got.Status != model.OrderStatusPending {
		t.Fatalf("order = %+v err = %v, want it left pending", got, err)
	}
	if f.gw.verifyReq.Amount != 1999 || f.gw.verifyReq.Currency != "USD" {
		t.Fatalf("verify request = %+v, want minor units and currency", f.gw.verifyReq)
	}
}
