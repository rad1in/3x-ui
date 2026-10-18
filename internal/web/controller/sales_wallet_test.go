package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestAdminWalletAdjustCreditsAndRefusesOverdraft(t *testing.T) {
	engine := newSalesEngine(t)
	ib := &model.Inbound{Tag: "adm-wallet", Enable: true, Port: 26004, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if _, err := (&service.ClientService{}).Create(&service.InboundService{}, &service.ClientCreatePayload{
		Client: model.Client{Email: "alice", Enable: true}, InboundIds: []int{ib.Id},
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}

	if msg := postJSON(t, engine, "/panel/api/sales/wallet/adjust", `{"email":"alice","currency":"USD","amount":500,"note":"gift"}`); !msg.Success {
		t.Fatalf("credit: %+v", msg)
	}
	msg := postJSON(t, engine, "/panel/api/sales/wallet/adjust", `{"email":"alice","currency":"USD","amount":-501}`)
	if msg.Success || !strings.Contains(msg.Msg, service.ErrWalletInsufficient) || strings.Contains(msg.Msg, "adjusted") {
		t.Fatalf("overdraft = %+v, want a failure naming %q", msg, service.ErrWalletInsufficient)
	}

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/sales/wallet?email=alice", nil))
	if !strings.Contains(w.Body.String(), `"amount":500`) || !strings.Contains(w.Body.String(), `"gift"`) {
		t.Fatalf("wallet view = %s, want the 500 balance and its ledger line", w.Body.String())
	}
}

func TestAdminDiscountSaveRefusesBadCodes(t *testing.T) {
	engine := newSalesEngine(t)
	msg := postJSON(t, engine, "/panel/api/sales/discounts/save", `{"code":"X","kind":"percent","value":10,"enabled":true}`)
	if msg.Success || strings.Contains(msg.Msg, "saved") {
		t.Fatalf("msg = %+v, a malformed code must not read as saved", msg)
	}
	if msg := postJSON(t, engine, "/panel/api/sales/discounts/save", `{"code":"spring25","kind":"percent","value":25,"enabled":true}`); !msg.Success {
		t.Fatalf("valid save: %+v", msg)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/sales/discounts", nil))
	if !strings.Contains(w.Body.String(), `"SPRING25"`) {
		t.Fatalf("list = %s, want the upper-cased code", w.Body.String())
	}
}
