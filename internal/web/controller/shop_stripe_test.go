package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

const testStripeWebhookSecret = "whsec_controller"

// stripeFake is a Stripe whose session reads back as configured.
type stripeFake struct {
	server  *httptest.Server
	session atomic.Value
	reads   atomic.Int32
}

type stripeHarness struct {
	*shopHarness
	fake *stripeFake
}

func newStripeHarness(t *testing.T) *stripeHarness {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	ib := &model.Inbound{Tag: "shop-stripe", Enable: true, Port: 26003, Protocol: model.VLESS, Settings: `{"clients":[]}`}
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
		PublicEnabled: true,
		PublicBaseURL: "https://shop.example.com",
		Stripe:        service.StripeConfig{Enabled: true, SecretKey: "sk_test_c", WebhookSecret: testStripeWebhookSecret},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("config: %v", err)
	}
	f := &stripeFake{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
			_, _ = w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.test/cs_test_1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/checkout/sessions/cs_test_1":
			f.reads.Add(1)
			_, _ = w.Write([]byte(f.session.Load().(string)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	orders := &service.OrderService{}
	orders.SetGatewayFactory(func(_ string, c *service.SalesConfig) payment.Starter {
		return &payment.StripeGateway{SecretKey: c.Stripe.SecretKey, BaseURL: f.server.URL}
	})
	engine := gin.New()
	newShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) }, orders)
	return &stripeHarness{shopHarness: &shopHarness{engine: engine, plan: plan}, fake: f}
}

func (h *stripeHarness) start(t *testing.T, email string) service.ShopOrderView {
	t.Helper()
	g := &gatewayHarness{shopHarness: h.shopHarness}
	return g.startWith(t, email, "stripe")
}

func (h *stripeHarness) paidSession(token string, amount int64) {
	h.fake.session.Store(`{"id":"cs_test_1","status":"complete","payment_status":"paid","amount_total":` +
		strconv.FormatInt(amount, 10) + `,"currency":"usd","client_reference_id":"` + token + `","payment_intent":"pi_1"}`)
}

func (h *stripeHarness) webhook(t *testing.T, body, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/shop/api/webhook/stripe", strings.NewReader(body))
	if signature != "" {
		req.Header.Set("Stripe-Signature", signature)
	}
	req.RemoteAddr = "203.0.113.60:1234"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	return w
}

func (h *stripeHarness) status(t *testing.T, token string) service.ShopOrderView {
	t.Helper()
	g := &gatewayHarness{shopHarness: h.shopHarness}
	return g.orderStatus(t, token)
}

func stripeEvent(token string) string {
	return `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","client_reference_id":"` + token + `","payment_status":"paid"}}}`
}

func stripeSignature(secret, body string, at time.Time) string {
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "." + body))
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestShopStripeWebhookDeliversOnceEvenWhenReplayed(t *testing.T) {
	h := newStripeHarness(t)
	view := h.start(t, "alice")
	if view.PayURL != "https://checkout.stripe.test/cs_test_1" || view.Status != model.OrderStatusPending {
		t.Fatalf("started order = %+v", view)
	}
	h.paidSession(view.Token, 1999)
	body := stripeEvent(view.Token)

	for i := 0; i < 2; i++ {
		if w := h.webhook(t, body, stripeSignature(testStripeWebhookSecret, body, time.Now())); w.Code != http.StatusOK {
			t.Fatalf("webhook #%d = %d %s", i, w.Code, w.Body.String())
		}
	}
	if got := h.status(t, view.Token); got.Status != model.OrderStatusPaid || !got.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}
	var clients int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Count(&clients)
	if clients != 1 {
		t.Fatalf("clients = %d, a replayed webhook must not deliver twice", clients)
	}
	if h.fake.reads.Load() != 1 {
		t.Fatalf("session reads = %d, the replay must not ask Stripe again", h.fake.reads.Load())
	}
}

func TestShopStripeWebhookWithABadSignatureMovesNothing(t *testing.T) {
	h := newStripeHarness(t)
	view := h.start(t, "mallory")
	h.paidSession(view.Token, 1999)
	body := stripeEvent(view.Token)

	for name, sig := range map[string]string{
		"none":   "",
		"forged": stripeSignature("whsec_other", body, time.Now()),
		"stale":  stripeSignature(testStripeWebhookSecret, body, time.Now().Add(-time.Hour)),
	} {
		if w := h.webhook(t, body, sig); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s signature = %d, want 401", name, w.Code)
		}
	}
	if got := h.status(t, view.Token); got.Status != model.OrderStatusPending || got.Fulfilled {
		t.Fatalf("order = %+v, a forged webhook must not move it", got)
	}
}

func TestShopStripeWebhookWithTheWrongAmountParksTheOrderForReview(t *testing.T) {
	h := newStripeHarness(t)
	view := h.start(t, "bob")
	h.paidSession(view.Token, 199)
	body := stripeEvent(view.Token)

	if w := h.webhook(t, body, stripeSignature(testStripeWebhookSecret, body, time.Now())); w.Code != http.StatusOK {
		t.Fatalf("webhook = %d %s", w.Code, w.Body.String())
	}
	if got := h.status(t, view.Token); got.Status != model.OrderStatusReview || got.Fulfilled {
		t.Fatalf("order = %+v, want parked for review and not delivered", got)
	}
}

func TestShopStripeWebhookClaimingPaymentOnAnUnpaidSessionDeliversNothing(t *testing.T) {
	h := newStripeHarness(t)
	view := h.start(t, "carol")
	h.fake.session.Store(`{"id":"cs_test_1","status":"open","payment_status":"unpaid","amount_total":1999,"currency":"usd","client_reference_id":"` + view.Token + `"}`)
	body := stripeEvent(view.Token)

	h.webhook(t, body, stripeSignature(testStripeWebhookSecret, body, time.Now()))
	if got := h.status(t, view.Token); got.Status != model.OrderStatusPending || got.Fulfilled {
		t.Fatalf("order = %+v, Stripe's own answer decides, not the event", got)
	}
}

func TestShopStripeReturnVerifiesAndCancelClosesTheOrder(t *testing.T) {
	h := newStripeHarness(t)
	g := &gatewayHarness{shopHarness: h.shopHarness}

	paid := h.start(t, "dave")
	h.paidSession(paid.Token, 1999)
	if w := g.callback(t, http.MethodGet, "/shop/api/callback/stripe?order="+paid.Token); w.Code != http.StatusSeeOther {
		t.Fatalf("return = %d, want a redirect", w.Code)
	}
	if got := h.status(t, paid.Token); got.Status != model.OrderStatusPaid || !got.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}

	canceled := h.start(t, "erin")
	reads := h.fake.reads.Load()
	g.callback(t, http.MethodGet, "/shop/api/callback/stripe?order="+canceled.Token+"&cancel=1")
	if got := h.status(t, canceled.Token); got.Status != model.OrderStatusFailed || h.fake.reads.Load() != reads {
		t.Fatalf("order = %+v after cancel, want failed without asking Stripe", got)
	}
}

func TestShopStripeLatePaymentAfterCancelIsParkedNotLost(t *testing.T) {
	h := newStripeHarness(t)
	g := &gatewayHarness{shopHarness: h.shopHarness}
	view := h.start(t, "frank")
	g.callback(t, http.MethodGet, "/shop/api/callback/stripe?order="+view.Token+"&cancel=1")

	h.paidSession(view.Token, 1999)
	body := stripeEvent(view.Token)
	h.webhook(t, body, stripeSignature(testStripeWebhookSecret, body, time.Now()))
	if got := h.status(t, view.Token); got.Status != model.OrderStatusReview || got.Fulfilled {
		t.Fatalf("order = %+v, want parked for review", got)
	}
}
