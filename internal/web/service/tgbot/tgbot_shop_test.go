package tgbot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// loadRealBotLocale installs the shipped en-US texts so the assertions read
// what a customer reads.
func loadRealBotLocale(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "translation", "en-US.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		for k, v := range node {
			switch x := v.(type) {
			case map[string]any:
				walk(prefix+k+".", x)
			case string:
				_ = bundle.AddMessages(language.MustParse("en-US"), &i18n.Message{ID: prefix + k, Other: x})
			}
		}
	}
	walk("", tree)
	orig := locale.LocalizerBot
	t.Cleanup(func() { locale.LocalizerBot = orig })
	locale.LocalizerBot = i18n.NewLocalizer(bundle, "en-US")
}

type shopBot struct {
	tb     *Tgbot
	planID int
	texts  func() []string
}

func newShopBot(t *testing.T) *shopBot {
	t.Helper()
	loadRealBotLocale(t)
	url, _, bodies := newBroadcastMock(t)
	swapTestBot(t, url)
	setBroadcastRunning(t, true)
	withAdmins(t, 1)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))

	ib := &model.Inbound{UserId: 1, Remark: "in", Port: 443, Protocol: model.VLESS, Enable: true, Tag: "in-443", Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{Name: "tpl", Group: "shop", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Monthly", Price: 250000, Currency: "IRT", TotalGB: 50 << 30, DurationDays: 30, LimitIP: 2, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	b := &shopBot{tb: &Tgbot{}, planID: plan.Id}
	b.texts = func() []string {
		var out []string
		for _, p := range bodies("sendMessage") {
			out = append(out, p["text"].(string))
		}
		return out
	}
	return b
}

func (b *shopBot) configure(t *testing.T, telegramShop bool) {
	t.Helper()
	cfg := &service.SalesConfig{TelegramShop: telegramShop, WalletEnabled: true, Card: service.CardPayConfig{
		Enabled: true, CardNumber: "6037991812345678", HolderName: "Ali Rezaei",
	}}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func (b *shopBot) tap(from int64, data string) {
	b.tb.answerShopCallback(&telego.CallbackQuery{
		ID: "q", From: telego.User{ID: from}, Data: data, Message: &telego.Message{Chat: telego.Chat{ID: from}},
	})
}

func (b *shopBot) lastText() string {
	texts := b.texts()
	if len(texts) == 0 {
		return ""
	}
	return texts[len(texts)-1]
}

func TestShopCommandAndCallbacksStaySilentForStrangersWhileOff(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, false)

	if _, ok := b.tb.gateCommand(commandFrom(777, "/shop")); ok {
		t.Error("a stranger's /shop was admitted while the shop is off")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "shop:plans"}); ok {
		t.Error("a stranger's shop button was admitted while the shop is off")
	}
	b.configure(t, true)
	if _, ok := b.tb.gateCommand(commandFrom(777, "/shop")); !ok {
		t.Error("a stranger's /shop was refused while the shop is on")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "shop:plans"}); !ok {
		t.Error("a stranger's shop button was refused while the shop is on")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "client_sub_links x"}); ok {
		t.Error("turning the shop on opened a non-shop button to strangers")
	}
}

func TestStrangerBuysByCardAndFollowsTheOrderToDelivery(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, true)
	id := strconv.Itoa(b.planID)

	b.tap(777, "shop:plans")
	if got := b.lastText(); !strings.Contains(got, "Choose a plan") {
		t.Fatalf("plans = %q", got)
	}
	b.tap(777, "shop:p:"+id)
	if got := b.lastText(); !strings.Contains(got, "<b>Monthly</b>") || !strings.Contains(got, "250000 IRT") {
		t.Fatalf("plan detail = %q", got)
	}
	b.tap(777, "shop:k:n")
	if got := b.lastText(); !strings.Contains(got, "tg777") || !strings.Contains(got, "Total: 250000 IRT") {
		t.Fatalf("confirm = %q", got)
	}
	b.tap(777, "shop:g:card")
	got := b.lastText()
	if !strings.Contains(got, "6037991812345678") || !strings.Contains(got, "Awaiting payment") {
		t.Fatalf("card order = %q, want the card details and the pending status", got)
	}

	orders, _ := (&service.OrderService{}).TelegramOrders(777, 5)
	if len(orders) != 1 {
		t.Fatalf("orders of 777 = %d, want 1", len(orders))
	}
	orderID := strconv.Itoa(orders[0].Id)

	b.tap(777, "shop:rc:"+orderID)
	if st, _ := userStateMgr.get(chatUser{chatID: 777, userID: 777}); st != shopStateReceipt+orderID {
		t.Fatalf("state = %q, want a pending receipt", st)
	}
	handled := b.tb.handleShopInput(&telego.Message{Text: "482913", Chat: telego.Chat{ID: 777}, From: &telego.User{ID: 777}}, chatUser{chatID: 777, userID: 777}, shopStateReceipt+orderID)
	if !handled {
		t.Fatal("receipt text was not consumed")
	}
	order, _ := (&service.OrderService{}).TelegramOrder(777, orders[0].Id)
	if order.Status != model.OrderStatusReview || order.ReceiptRef != "482913" {
		t.Fatalf("order = %s / %q, want review with the receipt", order.Status, order.ReceiptRef)
	}

	if _, err := (&service.OrderService{}).FulfilOrder(&b.tb.inboundService, orders[0].Id); err != nil {
		t.Fatal(err)
	}
	b.tap(777, "shop:o:"+orderID)
	all := strings.Join(b.texts(), "\n")
	if !strings.Contains(all, "Paid and activated") {
		t.Fatalf("no delivery message in %q", all)
	}
	if recs, _ := b.tb.clientService.GetRecordsByTgID(777); len(recs) != 1 {
		t.Fatalf("clients bound to 777 = %d, want the delivered one", len(recs))
	}
}

func TestShopOrderIsInvisibleToOtherAccounts(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, true)
	b.tap(777, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(777, "shop:k:n")
	b.tap(777, "shop:g:card")
	orders, _ := (&service.OrderService{}).TelegramOrders(777, 5)
	before := len(b.texts())

	b.tap(888, "shop:o:"+strconv.Itoa(orders[0].Id))
	texts := b.texts()[before:]
	if len(texts) != 1 || texts[0] != "Order not found." {
		t.Fatalf("foreign order reply = %q, want only the not-found refusal", texts)
	}
}

func TestShopRenewalNamesOnlyTheTappersOwnClient(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, true)
	seedClientRecord(t, "mine", "sub-mine", 555)
	seedClientRecord(t, "theirs", "sub-theirs", 666)
	var theirs model.ClientRecord
	database.GetDB().Where("email = ?", "theirs").First(&theirs)

	b.tap(555, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(555, "shop:k:r"+strconv.Itoa(theirs.Id))
	if got := b.lastText(); got != "There is no account with that username." {
		t.Fatalf("renewing a foreign client replied %q", got)
	}
	if orders, _ := (&service.OrderService{}).TelegramOrders(555, 5); len(orders) != 0 {
		t.Fatalf("a foreign renewal opened %d orders", len(orders))
	}
}
