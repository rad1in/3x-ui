package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// paypalFake is a PayPal whose order has a configurable state and which
// counts captures.
type paypalFake struct {
	server   *httptest.Server
	status   atomic.Value
	value    atomic.Value
	custom   atomic.Value
	captures atomic.Int32
}

func (f *paypalFake) orderBody(status string) string {
	body := `{"id":"ORD-1","status":"` + status + `","purchase_units":[{"custom_id":"` + f.custom.Load().(string) +
		`","amount":{"currency_code":"USD","value":"` + f.value.Load().(string) + `"}`
	if status == "COMPLETED" {
		body += `,"payments":{"captures":[{"id":"CAP-1","status":"COMPLETED","amount":{"currency_code":"USD","value":"` + f.value.Load().(string) + `"}}]}`
	}
	return body + `}]}`
}

func newPayPalHarness(t *testing.T) (*shopHarness, *paypalFake) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	ib := &model.Inbound{Tag: "shop-paypal", Enable: true, Port: 26004, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{Name: "t", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	plan, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Dollar", Price: 1999, Currency: "USD", TotalGB: 10 << 30, DurationDays: 30, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	cfg := &service.SalesConfig{
		PublicEnabled: true, PublicBaseURL: "https://shop.example.com",
		PayPal: service.PayPalConfig{Enabled: true, ClientId: "cid", ClientSecret: "secret", Sandbox: true},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("config: %v", err)
	}
	f := &paypalFake{}
	f.status.Store("CREATED")
	f.value.Store("19.99")
	f.custom.Store("")
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
		case "POST /v2/checkout/orders":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ORD-1","links":[{"href":"https://paypal.test/approve/ORD-1","rel":"payer-action"}]}`))
		case "GET /v2/checkout/orders/ORD-1":
			_, _ = w.Write([]byte(f.orderBody(f.status.Load().(string))))
		case "POST /v2/checkout/orders/ORD-1/capture":
			f.captures.Add(1)
			f.status.Store("COMPLETED")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(f.orderBody("COMPLETED")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	orders := &service.OrderService{}
	orders.SetGatewayFactory(func(_ string, c *service.SalesConfig) payment.Starter {
		return &payment.PayPalGateway{ClientID: c.PayPal.ClientId, ClientSecret: c.PayPal.ClientSecret, BaseURL: f.server.URL}
	})
	engine := gin.New()
	newShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) }, orders)
	return &shopHarness{engine: engine, plan: plan}, f
}

func TestShopPayPalReturnCapturesVerifiedOrderOnceAndReplayDeliversNothingTwice(t *testing.T) {
	h, f := newPayPalHarness(t)
	g := &gatewayHarness{shopHarness: h}
	view := g.startWith(t, "alice", "paypal")
	if view.PayURL != "https://paypal.test/approve/ORD-1" || view.Status != model.OrderStatusPending {
		t.Fatalf("started order = %+v", view)
	}
	f.custom.Store(view.Token)
	f.status.Store("APPROVED")

	for i := 0; i < 2; i++ {
		if w := g.callback(t, http.MethodGet, "/shop/api/callback/paypal?order="+view.Token+"&token=ORD-1&PayerID=P1"); w.Code != http.StatusSeeOther {
			t.Fatalf("return #%d = %d, want a redirect", i, w.Code)
		}
	}
	if got := g.orderStatus(t, view.Token); got.Status != model.OrderStatusPaid || !got.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}
	var clients int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Count(&clients)
	if clients != 1 || f.captures.Load() != 1 {
		t.Fatalf("clients = %d, captures = %d, want exactly one of each", clients, f.captures.Load())
	}
}

func TestShopPayPalReturnForAnOrderThatWasNotApprovedDeliversNothing(t *testing.T) {
	h, f := newPayPalHarness(t)
	g := &gatewayHarness{shopHarness: h}
	view := g.startWith(t, "mallory", "paypal")
	f.custom.Store(view.Token)

	g.callback(t, http.MethodGet, "/shop/api/callback/paypal?order="+view.Token+"&token=ORD-1&PayerID=forged")
	if got := g.orderStatus(t, view.Token); got.Status != model.OrderStatusPending || got.Fulfilled || f.captures.Load() != 0 {
		t.Fatalf("order = %+v captures = %d, a forged return must neither deliver nor capture", got, f.captures.Load())
	}
}

func TestShopPayPalApprovedOrderForTheWrongAmountIsNotCapturedAndNeedsReview(t *testing.T) {
	h, f := newPayPalHarness(t)
	g := &gatewayHarness{shopHarness: h}
	view := g.startWith(t, "bob", "paypal")
	f.custom.Store(view.Token)
	f.value.Store("0.99")
	f.status.Store("APPROVED")

	g.callback(t, http.MethodGet, "/shop/api/callback/paypal?order="+view.Token)
	if got := g.orderStatus(t, view.Token); got.Status != model.OrderStatusReview || got.Fulfilled || f.captures.Load() != 0 {
		t.Fatalf("order = %+v captures = %d, want parked for review without a capture", got, f.captures.Load())
	}
}

func TestShopPayPalCancelClosesTheOrderWithoutCapturing(t *testing.T) {
	h, f := newPayPalHarness(t)
	g := &gatewayHarness{shopHarness: h}
	view := g.startWith(t, "erin", "paypal")
	f.custom.Store(view.Token)
	f.status.Store("APPROVED")

	g.callback(t, http.MethodGet, "/shop/api/callback/paypal?order="+view.Token+"&cancel=1")
	if got := g.orderStatus(t, view.Token); got.Status != model.OrderStatusFailed || f.captures.Load() != 0 {
		t.Fatalf("order = %+v captures = %d, want failed and never captured", got, f.captures.Load())
	}
}
