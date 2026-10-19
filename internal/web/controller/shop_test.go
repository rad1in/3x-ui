package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type shopHarness struct {
	engine *gin.Engine
	plan   *model.Plan
}

func newShopHarness(t *testing.T, publicEnabled bool) *shopHarness {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)

	ib := &model.Inbound{Tag: "shop-in", Enable: true, Port: 26001, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{Name: "t", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	plan, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Monthly", Price: 100, Currency: "USD", DurationDays: 30, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	cfg := &service.SalesConfig{
		PublicEnabled: publicEnabled,
		Card:          service.CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("config: %v", err)
	}

	engine := gin.New()
	NewShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) })
	return &shopHarness{engine: engine, plan: plan}
}

type shopReply struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func (h *shopHarness) call(t *testing.T, method, path, body string) (int, shopReply) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:4444"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	var reply shopReply
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	return w.Code, reply
}

func TestShopIsInvisibleWhileSwitchedOff(t *testing.T) {
	h := newShopHarness(t, false)
	for _, path := range []string{"/shop/api/plans", "/shop/api/orders/abc"} {
		if code, _ := h.call(t, http.MethodGet, path, ""); code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404 while the shop is off", path, code)
		}
	}
	if code, _ := h.call(t, http.MethodPost, "/shop/api/orders", `{}`); code != http.StatusNotFound {
		t.Fatalf("POST orders = %d, want 404 while the shop is off", code)
	}
}

func TestShopCardOrderFromPurchaseToApproval(t *testing.T) {
	h := newShopHarness(t, true)

	code, reply := h.call(t, http.MethodGet, "/shop/api/plans", "")
	if code != http.StatusOK || !reply.Success || !strings.Contains(string(reply.Obj), `"Monthly"`) {
		t.Fatalf("plans = %d %+v", code, reply)
	}

	body := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"kind":"new","email":"Alice","gateway":"card","contact":"@alice"}`
	_, reply = h.call(t, http.MethodPost, "/shop/api/orders", body)
	if !reply.Success {
		t.Fatalf("create order: %+v", reply)
	}
	var view service.ShopOrderView
	if err := json.Unmarshal(reply.Obj, &view); err != nil {
		t.Fatalf("decode order view: %v", err)
	}
	if view.ClientEmail != "alice" || view.Status != model.OrderStatusPending || view.Card == nil || view.Card.CardNumber != "6037991812345678" {
		t.Fatalf("view = %+v, want a pending order for the lower-cased username showing the card", view)
	}

	_, reply = h.call(t, http.MethodPost, "/shop/api/orders/"+view.Token+"/receipt", `{"ref":"482913","note":"morning"}`)
	if !reply.Success {
		t.Fatalf("receipt: %+v", reply)
	}
	_, reply = h.call(t, http.MethodGet, "/shop/api/orders/"+view.Token, "")
	view = service.ShopOrderView{}
	_ = json.Unmarshal(reply.Obj, &view)
	if view.Status != model.OrderStatusReview {
		t.Fatalf("status = %q after the receipt, want review", view.Status)
	}

	var order model.Order
	if err := database.GetDB().Where("token = ?", view.Token).First(&order).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if _, err := (&service.OrderService{}).FulfilOrder(&service.InboundService{}, order.Id); err != nil {
		t.Fatalf("approve: %v", err)
	}
	_, reply = h.call(t, http.MethodGet, "/shop/api/orders/"+view.Token, "")
	view = service.ShopOrderView{}
	_ = json.Unmarshal(reply.Obj, &view)
	if view.Status != model.OrderStatusPaid || !view.Fulfilled || view.Card != nil {
		t.Fatalf("view after approval = %+v, want paid, fulfilled, no card", view)
	}
}

func TestShopTurnsRefusalsIntoCodes(t *testing.T) {
	h := newShopHarness(t, true)

	_, reply := h.call(t, http.MethodPost, "/shop/api/orders", `{"planId":`+strconv.Itoa(h.plan.Id)+`,"kind":"new","email":"x","gateway":"card"}`)
	if reply.Success || reply.Msg != service.ErrInvalidEmail {
		t.Fatalf("reply = %+v, want code %q", reply, service.ErrInvalidEmail)
	}
	_, reply = h.call(t, http.MethodGet, "/shop/api/orders/nonexistent", "")
	if reply.Success || reply.Msg != service.ErrOrderNotFound {
		t.Fatalf("reply = %+v, want code %q", reply, service.ErrOrderNotFound)
	}
	if code, _ := h.call(t, http.MethodPost, "/shop/api/orders", `{not json`); code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", code)
	}
}

func TestShopRateLimitsWrites(t *testing.T) {
	h := newShopHarness(t, true)
	body := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"kind":"new","email":"x","gateway":"card"}`
	var last int
	for range shopWriteLimit + 1 {
		last, _ = h.call(t, http.MethodPost, "/shop/api/orders", body)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("request %d = %d, want 429", shopWriteLimit+1, last)
	}
	if code, _ := h.call(t, http.MethodGet, "/shop/api/plans", ""); code != http.StatusOK {
		t.Fatalf("reads must not be limited, got %d", code)
	}
}

func TestWindowLimiterResetsAfterTheWindow(t *testing.T) {
	l := newWindowLimiter(2, time.Minute)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	if !l.allow("a") || !l.allow("a") || l.allow("a") {
		t.Fatal("want two calls allowed, third refused")
	}
	if !l.allow("b") {
		t.Fatal("another key has its own budget")
	}
	now = now.Add(time.Minute)
	if !l.allow("a") {
		t.Fatal("budget must come back after the window")
	}
}
