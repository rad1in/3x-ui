package tgbot

import (
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	shopCallbackPrefix = "shop:"
	shopStateCode      = "shop_code"
	shopStateReceipt   = "shop_receipt:"
	shopOrderListSize  = 5
	shopSelectionTTL   = 30 * time.Minute
)

// shopSelection is what one customer has picked so far; callback data is too
// short to carry it, so it is kept here per chat user.
type shopSelection struct {
	planID int
	kind   string
	email  string
	code   string
	at     time.Time
}

var (
	shopSelectionsMu sync.Mutex
	shopSelections   = map[chatUser]*shopSelection{}
)

func setShopSelection(actor chatUser, sel *shopSelection) {
	sel.at = time.Now()
	shopSelectionsMu.Lock()
	defer shopSelectionsMu.Unlock()
	for k, v := range shopSelections {
		if time.Since(v.at) > shopSelectionTTL {
			delete(shopSelections, k)
		}
	}
	shopSelections[actor] = sel
}

func getShopSelection(actor chatUser) *shopSelection {
	shopSelectionsMu.Lock()
	defer shopSelectionsMu.Unlock()
	sel := shopSelections[actor]
	if sel == nil || time.Since(sel.at) > shopSelectionTTL {
		return nil
	}
	copied := *sel
	return &copied
}

func (t *Tgbot) shopEnabled() bool {
	cfg, err := t.salesConfig.Get()
	return err == nil && cfg.TelegramShop
}

func isShopCallback(data string) bool { return strings.HasPrefix(data, shopCallbackPrefix) }

func money(amount int64, currency string) string {
	return strconv.FormatInt(amount, 10) + " " + currency
}

// shopError answers a refusal in the customer's words; anything that is not a
// customer-safe SalesError is logged and shown as the generic failure.
func (t *Tgbot) shopError(err error) string {
	if code := service.SalesErrorCode(err); code != "" {
		if text := t.I18nBot("shop.errors." + code); text != "shop.errors."+code {
			return text
		}
	}
	logger.Warning("tgbot shop:", err)
	return t.I18nBot("shop.errors.network")
}

func (t *Tgbot) shopMenuKeyboard() *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.plans")).WithCallbackData("shop:plans")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.orders")).WithCallbackData("shop:orders")),
	)
}

// handleShop answers /shop. While the shop is off a stranger gets nothing, as
// for every other command it is not allowed.
func (t *Tgbot) handleShop(chatId int64, from *telego.User) {
	if !t.shopEnabled() {
		if t.levelOf(from.ID) != levelStranger {
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.disabled"))
		}
		return
	}
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.menu"), t.shopMenuKeyboard())
}

// answerShopCallback serves every shop button. Each step re-checks the switch
// and derives the buyer from the tapping account, never from the data.
func (t *Tgbot) answerShopCallback(query *telego.CallbackQuery) {
	chatId := query.Message.GetChat().ID
	t.sendCallbackAnswerTgBot(query.ID, "")
	if !t.shopEnabled() {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.disabled"))
		return
	}
	actor := callbackActor(query)
	tgID := query.From.ID
	parts := strings.Split(strings.TrimPrefix(query.Data, shopCallbackPrefix), ":")
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	switch parts[0] {
	case "menu":
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.menu"), t.shopMenuKeyboard())
	case "plans":
		t.shopShowPlans(chatId)
	case "p":
		t.shopChoosePlan(chatId, tgID, actor, arg)
	case "k":
		t.shopChooseAccount(chatId, tgID, actor, arg)
	case "code":
		if getShopSelection(actor) == nil {
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.expired"))
			return
		}
		userStateMgr.set(actor, shopStateCode)
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.askCode"))
	case "g":
		t.shopCheckout(chatId, tgID, actor, arg)
	case "orders":
		t.shopShowOrders(chatId, tgID)
	case "o":
		t.shopShowOrder(chatId, tgID, arg)
	case "rc":
		id, err := strconv.Atoi(arg)
		if err != nil {
			return
		}
		if _, err := t.orderService.TelegramOrder(tgID, id); err != nil {
			t.SendMsgToTgbot(chatId, t.shopError(err))
			return
		}
		userStateMgr.set(actor, shopStateReceipt+arg)
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.askReceipt"))
	}
}

func (t *Tgbot) shopShowPlans(chatId int64) {
	catalog, err := t.orderService.Catalog()
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	if len(catalog.Plans) == 0 {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.noPlans"))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(catalog.Plans))
	for _, p := range catalog.Plans {
		label := p.Name + " - " + money(p.Price, p.Currency)
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).WithCallbackData("shop:p:"+strconv.Itoa(p.Id))))
	}
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.plansTitle"), tu.InlineKeyboard(rows...))
}

func (t *Tgbot) shopPlan(id int) (*service.ShopPlan, error) {
	catalog, err := t.orderService.Catalog()
	if err != nil {
		return nil, err
	}
	for i := range catalog.Plans {
		if catalog.Plans[i].Id == id {
			return &catalog.Plans[i], nil
		}
	}
	return nil, &service.SalesError{Code: service.ErrPlanUnavailable}
}

func (t *Tgbot) planDetail(p *service.ShopPlan) string {
	quota := t.I18nBot("tgbot.unlimited")
	if p.TotalGB > 0 {
		quota = common.FormatTraffic(p.TotalGB)
	}
	duration := t.I18nBot("tgbot.shop.noExpiry")
	if p.DurationDays > 0 {
		duration = strconv.Itoa(p.DurationDays) + " " + t.I18nBot("tgbot.days")
	}
	return t.I18nBot("tgbot.shop.planDetail",
		"Name=="+html.EscapeString(p.Name), "Description=="+html.EscapeString(p.Description),
		"Quota=="+quota, "Duration=="+duration, "Devices=="+strconv.Itoa(p.LimitIP),
		"Price=="+money(p.Price, p.Currency))
}

func (t *Tgbot) shopChoosePlan(chatId, tgID int64, actor chatUser, arg string) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	plan, err := t.shopPlan(id)
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	setShopSelection(actor, &shopSelection{planID: id})
	rows := [][]telego.InlineKeyboardButton{
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.newAccount")).WithCallbackData("shop:k:n")),
	}
	records, _ := t.clientService.GetRecordsByTgID(tgID)
	for _, rec := range records {
		label := t.I18nBot("tgbot.shop.buttons.renew", "Email=="+rec.Email)
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).WithCallbackData("shop:k:r"+strconv.Itoa(rec.Id))))
	}
	t.SendMsgToTgbot(chatId, t.planDetail(plan), tu.InlineKeyboard(rows...))
}

// shopChooseAccount fixes who the plan is for: a renewal must name one of the
// tapping account's own clients; a new purchase gets a generated username.
func (t *Tgbot) shopChooseAccount(chatId, tgID int64, actor chatUser, arg string) {
	sel := getShopSelection(actor)
	if sel == nil {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.expired"))
		return
	}
	if arg == "n" {
		name, err := t.orderService.TelegramUsername(tgID)
		if err != nil {
			t.SendMsgToTgbot(chatId, t.shopError(err))
			return
		}
		sel.kind, sel.email = model.OrderKindNew, name
	} else {
		recID, err := strconv.Atoi(strings.TrimPrefix(arg, "r"))
		rec, gerr := t.clientService.GetByID(recID)
		if err != nil || gerr != nil || rec.TgID != tgID {
			t.SendMsgToTgbot(chatId, t.shopError(&service.SalesError{Code: service.ErrEmailUnknown}))
			return
		}
		sel.kind, sel.email = model.OrderKindRenew, rec.Email
	}
	sel.code = ""
	setShopSelection(actor, sel)
	t.shopShowConfirm(chatId, sel)
}

var shopGatewayButtons = map[string]string{
	service.GatewayCard: "card", service.GatewayWallet: "wallet", service.GatewayZarinpal: "zarinpal",
	service.GatewayIDPay: "idpay", service.GatewayCrypto: "crypto",
}

func (t *Tgbot) shopShowConfirm(chatId int64, sel *shopSelection) {
	quote, err := t.orderService.Quote(service.QuoteInput{PlanId: sel.planID, DiscountCode: sel.code, Email: sel.email})
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	catalog, err := t.orderService.Catalog()
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	plan, err := t.shopPlan(sel.planID)
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	msg := t.I18nBot("tgbot.shop.confirm", "Plan=="+html.EscapeString(plan.Name), "Email=="+html.EscapeString(sel.email),
		"Amount=="+money(quote.Final, quote.Currency))
	if quote.Discount > 0 {
		msg += t.I18nBot("tgbot.shop.discountLine", "List=="+money(quote.List, quote.Currency), "Code=="+html.EscapeString(quote.DiscountCode))
	}
	rows := [][]telego.InlineKeyboardButton{}
	for _, gw := range catalog.Methods {
		if gw == service.GatewayWallet && sel.kind != model.OrderKindRenew {
			continue
		}
		label := t.I18nBot("tgbot.shop.buttons." + shopGatewayButtons[gw])
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).WithCallbackData("shop:g:"+gw)))
	}
	if len(rows) == 0 {
		t.SendMsgToTgbot(chatId, t.shopError(&service.SalesError{Code: service.ErrGatewayUnavailable}))
		return
	}
	rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.code")).WithCallbackData("shop:code")))
	t.SendMsgToTgbot(chatId, msg, tu.InlineKeyboard(rows...))
}

func (t *Tgbot) shopCheckout(chatId, tgID int64, actor chatUser, gateway string) {
	sel := getShopSelection(actor)
	if sel == nil || sel.kind == "" {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.expired"))
		return
	}
	order, needRestart, err := t.orderService.Checkout(&t.inboundService, service.CreateOrderInput{
		PlanId: sel.planID, Kind: sel.kind, Email: sel.email, Gateway: gateway, DiscountCode: sel.code, TgId: tgID,
	})
	if needRestart {
		t.xrayService.SetToNeedRestart()
	}
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	if order.FulfilledAt > 0 {
		websocket.BroadcastInvalidate(websocket.MessageTypeClients)
	}
	shopSelectionsMu.Lock()
	delete(shopSelections, actor)
	shopSelectionsMu.Unlock()
	t.sendShopOrder(chatId, order)
}

func (t *Tgbot) shopShowOrders(chatId, tgID int64) {
	orders, err := t.orderService.TelegramOrders(tgID, shopOrderListSize)
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	if len(orders) == 0 {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.noOrders"))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(orders))
	for _, o := range orders {
		label := "#" + strconv.Itoa(o.Id) + " " + o.PlanName + " - " + t.I18nBot("shop.statuses."+o.Status)
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).WithCallbackData("shop:o:"+strconv.Itoa(o.Id))))
	}
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.ordersTitle"), tu.InlineKeyboard(rows...))
}

func (t *Tgbot) shopShowOrder(chatId, tgID int64, arg string) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	order, err := t.orderService.TelegramOrder(tgID, id)
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	t.sendShopOrder(chatId, order)
}

// sendShopOrder shows an order the way the shop page does: card details or a
// payment link while payment is due, the subscription once it is delivered.
func (t *Tgbot) sendShopOrder(chatId int64, order *model.Order) {
	view, err := t.orderService.PublicView(order, "")
	if err != nil {
		t.SendMsgToTgbot(chatId, t.shopError(err))
		return
	}
	msg := t.I18nBot("tgbot.shop.order", "Id=="+strconv.Itoa(order.Id), "Plan=="+html.EscapeString(order.PlanName),
		"Amount=="+money(order.Amount, order.Currency), "Status=="+t.I18nBot("shop.statuses."+order.Status))
	rows := [][]telego.InlineKeyboardButton{}
	if view.Card != nil {
		msg += t.I18nBot("tgbot.shop.cardInfo", "Amount=="+money(order.Amount, order.Currency), "Card=="+html.EscapeString(view.Card.CardNumber),
			"Holder=="+html.EscapeString(view.Card.HolderName), "Bank=="+html.EscapeString(view.Card.BankName),
			"Instructions=="+html.EscapeString(view.Card.Instructions))
		if order.Status == model.OrderStatusPending || order.Status == model.OrderStatusReview {
			rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.receipt")).WithCallbackData("shop:rc:"+strconv.Itoa(order.Id))))
		}
	}
	if view.PayURL != "" {
		msg += t.I18nBot("tgbot.shop.payLink", "URL=="+html.EscapeString(view.PayURL))
	}
	if order.Status == model.OrderStatusRejected && order.RejectReason != "" {
		msg += "\r\n" + html.EscapeString(order.RejectReason)
	}
	rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("tgbot.shop.buttons.refresh")).WithCallbackData("shop:o:"+strconv.Itoa(order.Id))))
	t.SendMsgToTgbot(chatId, msg, tu.InlineKeyboard(rows...))
	if view.Fulfilled {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.delivered"))
		t.sendClientSubLinks(chatId, order.ClientEmail)
	}
}

// handleShopInput consumes the text a customer was asked for and reports
// whether the state was a shop one.
func (t *Tgbot) handleShopInput(message *telego.Message, actor chatUser, state string) bool {
	chatId := message.Chat.ID
	switch {
	case state == shopStateCode:
		userStateMgr.clear(actor)
		sel := getShopSelection(actor)
		if sel == nil || !t.shopEnabled() {
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.expired"))
			return true
		}
		sel.code = strings.TrimSpace(message.Text)
		if _, err := t.orderService.Quote(service.QuoteInput{PlanId: sel.planID, DiscountCode: sel.code, Email: sel.email}); err != nil {
			t.SendMsgToTgbot(chatId, t.shopError(err))
			return true
		}
		setShopSelection(actor, sel)
		t.shopShowConfirm(chatId, sel)
	case strings.HasPrefix(state, shopStateReceipt):
		userStateMgr.clear(actor)
		id, _ := strconv.Atoi(strings.TrimPrefix(state, shopStateReceipt))
		order, err := t.orderService.TelegramOrder(actor.userID, id)
		if err == nil {
			order, err = t.orderService.SubmitReceipt(order.Token, message.Text, "")
		}
		if err != nil {
			t.SendMsgToTgbot(chatId, t.shopError(err))
			return true
		}
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.shop.receiptSent"))
		t.sendShopOrder(chatId, order)
	default:
		return false
	}
	return true
}
