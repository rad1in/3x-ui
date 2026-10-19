package controller

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type walletHarness struct {
	*shopHarness
	subID string
}

func newWalletHarness(t *testing.T) *walletHarness {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)

	ib := &model.Inbound{Tag: "shop-wallet", Enable: true, Port: 26003, Protocol: model.VLESS, Settings: `{"clients":[]}`}
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
		PublicEnabled: true, WalletEnabled: true,
		Card:     service.CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
		Referral: service.ReferralConfig{Enabled: true, BonusPercent: 10},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := (&service.ClientService{}).Create(&service.InboundService{}, &service.ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true}, InboundIds: []int{ib.Id},
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	var rec model.ClientRecord
	if err := database.GetDB().Where("email = ?", "alice").First(&rec).Error; err != nil {
		t.Fatalf("load client: %v", err)
	}
	if _, err := (&service.WalletService{}).Adjust("alice", "IRT", 300000, "top up"); err != nil {
		t.Fatalf("credit: %v", err)
	}
	engine := gin.New()
	NewShopController(engine.Group("/shop"), func(c *gin.Context) { c.Status(http.StatusNotFound) })
	return &walletHarness{shopHarness: &shopHarness{engine: engine, plan: plan}, subID: rec.SubID}
}

func TestShopQuotePreviewsACouponWithoutReservingIt(t *testing.T) {
	h := newWalletHarness(t)
	if _, err := (&service.DiscountService{}).SaveDiscount(&model.DiscountCode{Code: "TEN", Kind: model.DiscountPercent, Value: 10, Enabled: true, MaxUses: 1}); err != nil {
		t.Fatalf("code: %v", err)
	}
	body := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"discountCode":"ten"}`
	for range 3 {
		_, reply := h.call(t, http.MethodPost, "/shop/api/quote", body)
		var q service.Quote
		if err := json.Unmarshal(reply.Obj, &q); err != nil || !reply.Success || q.Final != 225000 || q.Discount != 25000 {
			t.Fatalf("quote = %+v %+v, want 225000 every time: a preview must not use up a one-use code", reply, q)
		}
	}
	_, reply := h.call(t, http.MethodPost, "/shop/api/quote", `{"planId":`+strconv.Itoa(h.plan.Id)+`,"discountCode":"NOPE"}`)
	if reply.Success || reply.Msg != service.ErrDiscountInvalid {
		t.Fatalf("reply = %+v, want %q", reply, service.ErrDiscountInvalid)
	}
}

func TestShopWalletViewNeedsTheSubscriptionIdAndSharesAReferralCode(t *testing.T) {
	h := newWalletHarness(t)
	_, reply := h.call(t, http.MethodGet, "/shop/api/wallet?email=alice&proof=wrong", "")
	if reply.Success || reply.Msg != service.ErrProofInvalid {
		t.Fatalf("wrong proof = %+v, want %q", reply, service.ErrProofInvalid)
	}
	_, reply = h.call(t, http.MethodGet, "/shop/api/wallet?email=alice&proof="+url.QueryEscape(h.subID), "")
	var w service.ShopWallet
	if err := json.Unmarshal(reply.Obj, &w); err != nil || !reply.Success {
		t.Fatalf("wallet = %+v, %v", reply, err)
	}
	if len(w.Balances) != 1 || w.Balances[0].Amount != 300000 || len(w.ReferralCode) != 8 {
		t.Fatalf("wallet = %+v, want 300000 IRT and a referral code", w)
	}
}

func TestShopPaysARenewalFromTheWalletAndShowsTheDeliveredOrder(t *testing.T) {
	h := newWalletHarness(t)
	body := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"kind":"renew","email":"alice","gateway":"wallet","proof":"` + h.subID + `"}`
	code, reply := h.call(t, http.MethodPost, "/shop/api/orders", body)
	if code != http.StatusOK || !reply.Success {
		t.Fatalf("wallet order = %d %+v", code, reply)
	}
	var view service.ShopOrderView
	if err := json.Unmarshal(reply.Obj, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != model.OrderStatusPaid || !view.Fulfilled {
		t.Fatalf("order = %+v, want paid and delivered at once", view)
	}
	bal, _ := (&service.WalletService{}).Balances("alice")
	if len(bal) != 1 || bal[0].Amount != 50000 {
		t.Fatalf("balances = %+v, want 300000 - 250000 left", bal)
	}

	_, reply = h.call(t, http.MethodPost, "/shop/api/orders", body)
	if reply.Success || reply.Msg != service.ErrWalletInsufficient {
		t.Fatalf("second payment = %+v, want %q", reply, service.ErrWalletInsufficient)
	}
}
