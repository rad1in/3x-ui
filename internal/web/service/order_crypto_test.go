package service

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

const ipnSecret = "ipn-secret-for-tests"

type cryptoFixture struct {
	*shopFixture
	gw *stubGateway
}

func newCryptoFixture(t *testing.T, port int) *cryptoFixture {
	t.Helper()
	f := newShopFixture(t, port)
	usd := *f.plan
	usd.Id, usd.Name, usd.Currency, usd.Price = 0, "Dollar plan", "USD", 1999
	plan, err := (&PlanService{}).SavePlan(&usd)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	f.plan = plan
	cfg := &SalesConfig{
		PublicBaseURL: "https://shop.example.com",
		Crypto:        CryptoConfig{Enabled: true, ApiKey: "np-key", IpnSecret: ipnSecret},
	}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	gw := &stubGateway{}
	f.orders.gatewayFactory = func(string, *SalesConfig) payment.Starter { return gw }
	return &cryptoFixture{shopFixture: f, gw: gw}
}

func (f *cryptoFixture) cryptoOrder(t *testing.T, email string) *model.Order {
	t.Helper()
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: email, Gateway: GatewayCrypto})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	o, err = f.orders.StartPayment(o)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	return o
}

// ipnBody builds a webhook the way the sender does, with its signature over
// the key-sorted text (keys are written already sorted here on purpose).
func ipnBody(token, status, price, currency string) (string, string) {
	body := fmt.Sprintf(`{"actually_paid":%s,"order_id":%q,"pay_amount":19.99,"payment_id":5077125051,"payment_status":%q,"price_amount":%s,"price_currency":%q}`,
		price, token, status, price, currency)
	mac := hmac.New(sha512.New, []byte(ipnSecret))
	mac.Write([]byte(body))
	return body, hex.EncodeToString(mac.Sum(nil))
}

func TestCryptoOrderStartsAnInvoiceInThePlanCurrency(t *testing.T) {
	f := newCryptoFixture(t, 28001)
	o := f.cryptoOrder(t, "alice")

	req := f.gw.startReq
	if req.Amount != 1999 || req.Currency != "USD" {
		t.Fatalf("start request = %+v, want 1999 minor units of USD, not rials", req)
	}
	if req.CallbackURL != "https://shop.example.com/shop/api/ipn/nowpayments" || req.ReturnURL != "https://shop.example.com/shop/?order="+o.Token {
		t.Fatalf("urls = %q / %q", req.CallbackURL, req.ReturnURL)
	}
	if want := fmt.Sprintf("Order %d", o.Id); req.Description != want {
		t.Fatalf("description = %q, want the ASCII %q so webhook signatures stay unambiguous", req.Description, want)
	}
	if o.PayURL == "" || o.GatewayRef != "AUTH-1" {
		t.Fatalf("order = %+v, want the invoice stored", o)
	}
}

func TestCryptoOrderRefusesCurrenciesTheInvoiceCannotUse(t *testing.T) {
	f := newCryptoFixture(t, 28002)
	toman := *f.plan
	toman.Id, toman.Name, toman.Currency = 0, "Toman plan", "IRT"
	tp, err := (&PlanService{}).SavePlan(&toman)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	_, err = f.orders.CreateOrder(CreateOrderInput{PlanId: tp.Id, Kind: model.OrderKindNew, Email: "alice", Gateway: GatewayCrypto})
	wantSalesError(t, err, ErrCurrencyUnsupported)
}

func TestFinishedIPNDeliversOnceAndIsIdempotent(t *testing.T) {
	f := newCryptoFixture(t, 28003)
	o := f.cryptoOrder(t, "alice")
	body, sig := ipnBody(o.Token, "finished", "19.99", "usd")

	got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	if err != nil {
		t.Fatalf("HandleCryptoIPN: %v", err)
	}
	if got.Status != model.OrderStatusPaid || got.FulfilledAt == 0 || got.ReceiptRef != "5077125051" {
		t.Fatalf("order = %+v, want paid, delivered and carrying the payment id", got)
	}
	again, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	if err != nil || again.Status != model.OrderStatusPaid {
		t.Fatalf("repeated IPN = %+v, %v; want an idempotent success", again, err)
	}
	if clientCount(t, "alice") != 1 {
		t.Fatalf("%d clients, want 1", clientCount(t, "alice"))
	}
}

func TestIPNWithABadSignatureChangesNothing(t *testing.T) {
	f := newCryptoFixture(t, 28004)
	o := f.cryptoOrder(t, "alice")
	body, sig := ipnBody(o.Token, "finished", "19.99", "usd")

	for name, signature := range map[string]string{"missing": "", "wrong": sig[:len(sig)-2] + "00", "garbage": "nope"} {
		_, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), signature)
		wantSalesError(t, err, ErrBadSignature)
		if after, _ := f.orders.GetOrder(o.Id); after.Status != model.OrderStatusPending || clientCount(t, "alice") != 0 {
			t.Fatalf("%s signature: order = %+v; an unsigned webhook must not move an order", name, after)
		}
	}
}

func TestIPNForAnotherPriceIsNotAccepted(t *testing.T) {
	f := newCryptoFixture(t, 28005)
	o := f.cryptoOrder(t, "alice")
	cases := map[string][2]string{"cheaper": {"0.50", "usd"}, "other currency": {"19.99", "eur"}, "sub-cent": {"19.999", "usd"}}
	for name, c := range cases {
		body, sig := ipnBody(o.Token, "finished", c[0], c[1])
		_, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
		wantSalesError(t, err, ErrAmountMismatch)
		if clientCount(t, "alice") != 0 {
			t.Fatalf("%s: a mismatched payment delivered a client", name)
		}
	}
}

func TestIPNMovesThroughTheInProgressStatusesWithoutDelivering(t *testing.T) {
	f := newCryptoFixture(t, 28006)
	o := f.cryptoOrder(t, "alice")
	for _, status := range []string{"waiting", "confirming", "confirmed", "sending"} {
		body, sig := ipnBody(o.Token, status, "19.99", "usd")
		got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
		if err != nil || got.Status != model.OrderStatusPending || clientCount(t, "alice") != 0 {
			t.Fatalf("%s: order = %+v, %v; want it left pending without a client", status, got, err)
		}
	}
}

func TestPartialPaymentIsParkedForTheAdmin(t *testing.T) {
	f := newCryptoFixture(t, 28007)
	o := f.cryptoOrder(t, "alice")
	body, sig := ipnBody(o.Token, "partially_paid", "19.99", "usd")
	got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	if err != nil || got.Status != model.OrderStatusReview || got.ReceiptNote == "" || clientCount(t, "alice") != 0 {
		t.Fatalf("order = %+v, %v; want review with a note and no client", got, err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("the admin could not approve the parked order: %v", err)
	}
	if clientCount(t, "alice") != 1 {
		t.Fatal("approval after review did not deliver")
	}
}

func TestFailedAndExpiredIPNCloseTheOrder(t *testing.T) {
	for _, status := range []string{"failed", "expired"} {
		f := newCryptoFixture(t, 28008)
		o := f.cryptoOrder(t, "alice")
		body, sig := ipnBody(o.Token, status, "19.99", "usd")
		got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
		if err != nil || got.Status != model.OrderStatusFailed {
			t.Fatalf("%s: order = %+v, %v; want failed", status, got, err)
		}
	}
}

func TestLatePaymentOnACanceledOrderIsNotLost(t *testing.T) {
	f := newCryptoFixture(t, 28009)
	o := f.cryptoOrder(t, "alice")
	old := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	if err := database.GetDB().Model(&model.Order{}).Where("id = ?", o.Id).Update("created_at", old).Error; err != nil {
		t.Fatalf("age order: %v", err)
	}
	if _, _, err := f.orders.ReconcileOnline(&InboundService{}); err != nil {
		t.Fatalf("ReconcileOnline: %v", err)
	}
	if after, _ := f.orders.GetOrder(o.Id); after.Status != model.OrderStatusCanceled {
		t.Fatalf("status = %q, want the week-old unpaid crypto order canceled", after.Status)
	}

	body, sig := ipnBody(o.Token, "finished", "19.99", "usd")
	got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	if err != nil || got.Status != model.OrderStatusReview || clientCount(t, "alice") != 0 {
		t.Fatalf("late payment: order = %+v, %v; want it parked in review rather than dropped or auto-delivered", got, err)
	}
}

func TestIPNRefusesOtherOrdersAndMethods(t *testing.T) {
	f := newCryptoFixture(t, 28010)
	body, sig := ipnBody("no-such-token", "finished", "19.99", "usd")
	_, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	wantSalesError(t, err, ErrOrderNotFound)

	if err := (&SalesConfigService{}).Save(&SalesConfig{
		PublicBaseURL: "https://shop.example.com",
		Crypto:        CryptoConfig{Enabled: true, ApiKey: "np-key", IpnSecret: ipnSecret},
		Card:          CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
	}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	card, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "carl", Gateway: GatewayCard})
	if err != nil {
		t.Fatalf("CreateOrder card: %v", err)
	}
	body, sig = ipnBody(card.Token, "finished", "19.99", "usd")
	_, _, err = f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	wantSalesError(t, err, ErrOrderState)
	if clientCount(t, "carl") != 0 {
		t.Fatal("a crypto webhook delivered a card order")
	}
}

func TestIPNIsRefusedWhileCryptoIsOff(t *testing.T) {
	f := newCryptoFixture(t, 28011)
	o := f.cryptoOrder(t, "alice")
	if err := (&SalesConfigService{}).Save(&SalesConfig{}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	body, sig := ipnBody(o.Token, "finished", "19.99", "usd")
	_, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	wantSalesError(t, err, ErrGatewayUnavailable)
}

func TestDecimalToMinor(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"19.99", 1999, true}, {"20", 2000, true}, {"0.1", 10, true}, {"1.10", 110, true}, {"19.999", 0, false},
		{"-1", 0, false}, {"abc", 0, false}, {"", 0, false}, {"1e2", 10000, true},
	}
	for _, tc := range cases {
		got, ok := decimalToMinor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("decimalToMinor(%q) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReconcileLeavesAFreshCryptoOrderAlone(t *testing.T) {
	f := newCryptoFixture(t, 28012)
	o := f.cryptoOrder(t, "alice")
	if _, _, err := f.orders.ReconcileOnline(&InboundService{}); err != nil {
		t.Fatalf("ReconcileOnline: %v", err)
	}
	if after, _ := f.orders.GetOrder(o.Id); after.Status != model.OrderStatusPending {
		t.Fatalf("status = %q, a crypto customer may still be sending coins", after.Status)
	}
}

func TestRepeatedFinishedIPNRetriesAFailedDelivery(t *testing.T) {
	f := newCryptoFixture(t, 28013)
	o := f.cryptoOrder(t, "alice")
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed colliding client: %v", err)
	}
	body, sig := ipnBody(o.Token, "finished", "19.99", "usd")
	if got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig); err == nil || got.Status != model.OrderStatusPaid || got.FulfilledAt != 0 {
		t.Fatalf("first IPN = %+v, %v; want a paid order whose delivery failed", got, err)
	}
	if _, err := (&ClientService{}).DeleteByEmail(&InboundService{}, "alice", false); err != nil {
		t.Fatalf("remove collision: %v", err)
	}
	got, _, err := f.orders.HandleCryptoIPN(&InboundService{}, []byte(body), sig)
	if err != nil || got.FulfilledAt == 0 || clientCount(t, "alice") != 1 {
		t.Fatalf("retry = %+v, %v; want the delivery completed", got, err)
	}
}
