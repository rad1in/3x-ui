package controller

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
)

// zarinpalFake is a Zarinpal that answers verify with a configurable code.
type zarinpalFake struct {
	server      *httptest.Server
	verifyBody  atomic.Value
	verifyCalls atomic.Int32
}

func newZarinpalFake(t *testing.T) *zarinpalFake {
	t.Helper()
	f := &zarinpalFake{}
	f.verifyBody.Store(`{"data":{"code":100,"message":"Verified","ref_id":4242},"errors":[]}`)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pg/v4/payment/request.json":
			_, _ = w.Write([]byte(`{"data":{"code":100,"message":"Success","authority":"A000000000000000000000000000000000AB"},"errors":[]}`))
		case "/pg/v4/payment/verify.json":
			f.verifyCalls.Add(1)
			_, _ = w.Write([]byte(f.verifyBody.Load().(string)))
		case "/v1/invoice":
			_, _ = w.Write([]byte(`{"id":"inv-1","invoice_url":"https://nowpayments.test/payment/?iid=inv-1"}`))
		case "/v1.1/payment":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"idpay-id-1","link":"https://idpay.test/p/ws/idpay-id-1"}`))
		case "/v1.1/payment/verify":
			f.verifyCalls.Add(1)
			var in struct {
				OrderID string `json:"order_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			_, _ = w.Write([]byte(`{"status":100,"track_id":"777","order_id":"` + in.OrderID + `","amount":2500000}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

type gatewayHarness struct {
	*shopHarness
	fake    *zarinpalFake
	usdPlan *model.Plan
}

const testIPNSecret = "controller-ipn-secret"

func newGatewayHarness(t *testing.T) *gatewayHarness {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	gin.SetMode(gin.TestMode)

	ib := &model.Inbound{Tag: "shop-gw", Enable: true, Port: 26002, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{Name: "t", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	plan, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Monthly", Price: 250000, Currency: "IRT", TotalGB: 10 << 30, DurationDays: 30, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	cfg := &service.SalesConfig{
		PublicEnabled: true,
		PublicBaseURL: "https://shop.example.com",
		Zarinpal:      service.ZarinpalConfig{Enabled: true, MerchantId: "merchant"},
		IDPay:         service.IDPayConfig{Enabled: true, ApiKey: "idpay-key"},
		Crypto:        service.CryptoConfig{Enabled: true, ApiKey: "np-key", IpnSecret: testIPNSecret},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("config: %v", err)
	}

	fake := newZarinpalFake(t)
	orders := &service.OrderService{}
	orders.SetGatewayFactory(func(name string, c *service.SalesConfig) payment.Starter {
		if name == service.GatewayCrypto {
			return &payment.NOWPaymentsGateway{APIKey: c.Crypto.ApiKey, BaseURL: fake.server.URL}
		}
		if name == service.GatewayIDPay {
			return &payment.IDPayGateway{APIKey: c.IDPay.ApiKey, BaseURL: fake.server.URL}
		}
		return &payment.ZarinpalGateway{MerchantID: c.Zarinpal.MerchantId, BaseURL: fake.server.URL}
	})
	engine := gin.New()
	newShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) }, orders)
	usd, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Dollar", Price: 1999, Currency: "USD", TotalGB: 10 << 30, DurationDays: 30, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatalf("usd plan: %v", err)
	}
	return &gatewayHarness{shopHarness: &shopHarness{engine: engine, plan: plan}, fake: fake, usdPlan: usd}
}

func (h *gatewayHarness) start(t *testing.T, email string) service.ShopOrderView {
	t.Helper()
	return h.startWith(t, email, "zarinpal")
}

func (h *gatewayHarness) startWith(t *testing.T, email, gateway string) service.ShopOrderView {
	t.Helper()
	body := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"kind":"new","email":"` + email + `","gateway":"` + gateway + `"}`
	code, reply := h.call(t, http.MethodPost, "/shop/api/orders", body)
	if code != http.StatusOK || !reply.Success {
		t.Fatalf("create order = %d %+v", code, reply)
	}
	var view service.ShopOrderView
	if err := json.Unmarshal(reply.Obj, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

func (h *gatewayHarness) callback(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	return w
}

func (h *gatewayHarness) orderStatus(t *testing.T, token string) service.ShopOrderView {
	t.Helper()
	_, reply := h.call(t, http.MethodGet, "/shop/api/orders/"+token, "")
	var view service.ShopOrderView
	if err := json.Unmarshal(reply.Obj, &view); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	return view
}

func TestShopZarinpalPaymentFromStartToDeliveredClient(t *testing.T) {
	h := newGatewayHarness(t)
	view := h.start(t, "alice")
	if view.Status != model.OrderStatusPending || !strings.HasSuffix(view.PayURL, "/pg/StartPay/A000000000000000000000000000000000AB") {
		t.Fatalf("started order = %+v, want pending with the Zarinpal pay url", view)
	}

	w := h.callback(t, http.MethodGet, "/shop/api/callback/zarinpal?order="+view.Token+"&Authority=A000000000000000000000000000000000AB&Status=OK")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://shop.example.com/shop/?order="+view.Token {
		t.Fatalf("callback = %d -> %q, want a redirect to the shop order page", w.Code, w.Header().Get("Location"))
	}
	got := h.orderStatus(t, view.Token)
	if got.Status != model.OrderStatusPaid || !got.Fulfilled || got.PayURL != "" {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}
	var n int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Count(&n)
	if n != 1 {
		t.Fatalf("%d clients for alice, want 1", n)
	}
}

func TestShopCallbackWithForgedSuccessIsNotBelieved(t *testing.T) {
	h := newGatewayHarness(t)
	h.fake.verifyBody.Store(`{"data":[],"errors":{"code":-51,"message":"Session is not valid"}}`)
	view := h.start(t, "mallory")

	h.callback(t, http.MethodGet, "/shop/api/callback/zarinpal?order="+view.Token+"&Status=OK")
	got := h.orderStatus(t, view.Token)
	if got.Status != model.OrderStatusFailed || got.Fulfilled {
		t.Fatalf("order = %+v, want failed: Status=OK alone must never deliver", got)
	}
	var n int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "mallory").Count(&n)
	if n != 0 {
		t.Fatalf("a forged callback created %d clients", n)
	}
}

func TestShopCallbackCancelledByCustomerFailsWithoutVerifying(t *testing.T) {
	h := newGatewayHarness(t)
	view := h.start(t, "alice")
	h.callback(t, http.MethodGet, "/shop/api/callback/zarinpal?order="+view.Token+"&Status=NOK")
	if got := h.orderStatus(t, view.Token); got.Status != model.OrderStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if h.fake.verifyCalls.Load() != 0 {
		t.Fatal("a cancelled payment must not be verified")
	}
}

func TestShopCallbackUnknownGatewayAndTokenAreRefused(t *testing.T) {
	h := newGatewayHarness(t)
	if w := h.callback(t, http.MethodGet, "/shop/api/callback/paypal?order=x"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown gateway = %d, want 404", w.Code)
	}
	if w := h.callback(t, http.MethodGet, "/shop/api/callback/zarinpal?order=nope&Status=OK"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown order = %d, want 404 and no redirect that confirms anything", w.Code)
	}
}

func TestShopIDPayPostedCallbackDeliversTheClient(t *testing.T) {
	h := newGatewayHarness(t)
	view := h.startWith(t, "idpay-user", "idpay")
	if view.PayURL != "https://idpay.test/p/ws/idpay-id-1" {
		t.Fatalf("pay url = %q", view.PayURL)
	}

	req := httptest.NewRequest(http.MethodPost, "/shop/api/callback/idpay?order="+view.Token, strings.NewReader("status=10&track_id=777&id=idpay-id-1&order_id="+view.Token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback = %d, want a redirect", w.Code)
	}
	if got := h.orderStatus(t, view.Token); got.Status != model.OrderStatusPaid || !got.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}
}

func (h *gatewayHarness) startCrypto(t *testing.T, email string) service.ShopOrderView {
	t.Helper()
	body := `{"planId":` + strconv.Itoa(h.usdPlan.Id) + `,"kind":"new","email":"` + email + `","gateway":"crypto"}`
	code, reply := h.call(t, http.MethodPost, "/shop/api/orders", body)
	if code != http.StatusOK || !reply.Success {
		t.Fatalf("create crypto order = %d %+v", code, reply)
	}
	var view service.ShopOrderView
	if err := json.Unmarshal(reply.Obj, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

func (h *gatewayHarness) ipn(t *testing.T, body, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/shop/api/ipn/nowpayments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("x-nowpayments-sig", signature)
	}
	req.RemoteAddr = "203.0.113.50:1234"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	return w
}

// signedIPN signs the key-sorted body the way NOWPayments does.
func signedIPN(token, status string) (string, string) {
	body := `{"order_id":"` + token + `","payment_id":99,"payment_status":"` + status + `","price_amount":19.99,"price_currency":"usd"}`
	mac := hmac.New(sha512.New, []byte(testIPNSecret))
	mac.Write([]byte(body))
	return body, hex.EncodeToString(mac.Sum(nil))
}

func TestShopCryptoOrderFromInvoiceToSignedWebhook(t *testing.T) {
	h := newGatewayHarness(t)
	view := h.startCrypto(t, "alice")
	if view.PayURL != "https://nowpayments.test/payment/?iid=inv-1" || view.Status != model.OrderStatusPending {
		t.Fatalf("started order = %+v, want the hosted invoice link", view)
	}

	body, sig := signedIPN(view.Token, "finished")
	if w := h.ipn(t, body, sig); w.Code != http.StatusOK {
		t.Fatalf("signed webhook = %d %s, want 200", w.Code, w.Body.String())
	}
	if got := h.orderStatus(t, view.Token); got.Status != model.OrderStatusPaid || !got.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered", got)
	}
}

func TestShopCryptoWebhookWithoutAValidSignatureIsRefused(t *testing.T) {
	h := newGatewayHarness(t)
	view := h.startCrypto(t, "mallory")
	body, _ := signedIPN(view.Token, "finished")

	for name, sig := range map[string]string{"none": "", "forged": strings.Repeat("ab", 64)} {
		if w := h.ipn(t, body, sig); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s signature = %d, want 401", name, w.Code)
		}
	}
	if got := h.orderStatus(t, view.Token); got.Status != model.OrderStatusPending || got.Fulfilled {
		t.Fatalf("order = %+v, a forged webhook must not move it", got)
	}
}

func TestShopCryptoWebhookStopsRetriesForOrdersItCannotUse(t *testing.T) {
	h := newGatewayHarness(t)
	body, sig := signedIPN("unknown-token", "finished")
	w := h.ipn(t, body, sig)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), service.ErrOrderNotFound) {
		t.Fatalf("webhook for an unknown order = %d %s, want 200 so the sender stops retrying", w.Code, w.Body.String())
	}
}
