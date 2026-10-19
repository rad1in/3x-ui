package controller

import (
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// SalesController is the admin side of selling plans: the plan catalogue and
// the order queue.
type SalesController struct {
	planService    service.PlanService
	orderService   service.OrderService
	configService  service.SalesConfigService
	discounts      service.DiscountService
	wallets        service.WalletService
	tickets        service.TicketService
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewSalesController(g *gin.RouterGroup) *SalesController {
	a := &SalesController{}
	g.GET("/plans", a.listPlans)
	g.POST("/plans/save", a.savePlan)
	g.POST("/plans/del/:id", a.deletePlan)
	g.GET("/orders", a.listOrders)
	g.GET("/orders/export", a.exportOrders)
	g.POST("/orders/approve/:id", a.approveOrder)
	g.POST("/orders/reject/:id", a.rejectOrder)
	g.GET("/discounts", a.listDiscounts)
	g.POST("/discounts/save", a.saveDiscount)
	g.POST("/discounts/del/:id", a.deleteDiscount)
	g.GET("/wallet", a.getWallet)
	g.POST("/wallet/adjust", a.adjustWallet)
	g.GET("/tickets", a.listTickets)
	g.GET("/tickets/:id", a.getTicket)
	g.POST("/tickets/reply/:id", a.replyTicket)
	g.POST("/tickets/close/:id", a.closeTicket)
	g.POST("/tickets/reopen/:id", a.reopenTicket)
	g.GET("/config", a.getConfig)
	g.POST("/config/save", a.saveConfig)
	return a
}

// respondDone answers with the success message, or with a failure that does
// not carry it: jsonMsg would prefix the error with the success text.
func respondDone(c *gin.Context, okKey string, obj any, err error) {
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, okKey), obj, nil)
}

func (a *SalesController) listPlans(c *gin.Context) {
	rows, err := a.planService.ListPlans(false)
	jsonObj(c, rows, err)
}

func (a *SalesController) savePlan(c *gin.Context) {
	var plan model.Plan
	if err := c.ShouldBindJSON(&plan); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	saved, err := a.planService.SavePlan(&plan)
	respondDone(c, "pages.sales.plans.saved", saved, err)
}

func (a *SalesController) deletePlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.sales.plans.deleted", nil, a.planService.DeletePlan(id))
}

func (a *SalesController) listOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	page, err := a.orderService.ListOrders(c.Query("status"), limit, offset)
	jsonObj(c, page, err)
}

func (a *SalesController) exportOrders(c *gin.Context) {
	format := c.DefaultQuery("format", "xlsx")
	if !isTableFormat(format) {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), nil)
		return
	}
	status := c.Query("status")
	orders, err := a.orderService.ExportOrders(status)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	now := time.Now()
	serveTable(c, format, "orders-"+now.Format("2006-01-02"), service.OrdersTable(orders, status, now))
}

func (a *SalesController) approveOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	needRestart, err := a.orderService.FulfilOrder(&a.inboundService, id)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	notifyClientsChanged()
	order, err := a.orderService.GetOrder(id)
	respondDone(c, "pages.sales.orders.approved", order, err)
}

type rejectOrderBody struct {
	Reason string `json:"reason"`
}

func (a *SalesController) rejectOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var body rejectOrderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	order, err := a.orderService.RejectOrder(id, body.Reason)
	respondDone(c, "pages.sales.orders.rejected", order, err)
}

func (a *SalesController) getConfig(c *gin.Context) {
	view, err := a.configService.GetView()
	jsonObj(c, view, err)
}

func (a *SalesController) saveConfig(c *gin.Context) {
	var cfg service.SalesConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.sales.config.saved", nil, a.configService.Save(&cfg))
}

func (a *SalesController) listDiscounts(c *gin.Context) {
	rows, err := a.discounts.ListDiscounts()
	jsonObj(c, rows, err)
}

func (a *SalesController) saveDiscount(c *gin.Context) {
	var code model.DiscountCode
	if err := c.ShouldBindJSON(&code); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	saved, err := a.discounts.SaveDiscount(&code)
	respondDone(c, "pages.sales.discounts.saved", saved, err)
}

func (a *SalesController) deleteDiscount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.sales.discounts.deleted", nil, a.discounts.DeleteDiscount(id))
}

func (a *SalesController) getWallet(c *gin.Context) {
	view, err := a.wallets.View(c.Query("email"))
	jsonObj(c, view, err)
}

type adjustWalletBody struct {
	Email    string `json:"email"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
	Note     string `json:"note"`
}

func (a *SalesController) adjustWallet(c *gin.Context) {
	var body adjustWalletBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	entry, err := a.wallets.Adjust(body.Email, body.Currency, body.Amount, body.Note)
	respondDone(c, "pages.sales.wallets.adjusted", entry, err)
}

func (a *SalesController) listTickets(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	page, err := a.tickets.List(c.Query("status"), limit, offset)
	jsonObj(c, page, err)
}

func (a *SalesController) getTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	detail, err := a.tickets.Get(id)
	jsonObj(c, detail, err)
}

type replyTicketBody struct {
	Body string `json:"body"`
}

func (a *SalesController) replyTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var body replyTicketBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	detail, err := a.tickets.AdminReply(id, body.Body)
	respondDone(c, "pages.sales.tickets.replied", detail, err)
}

func (a *SalesController) closeTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	ticket, err := a.tickets.Close(id)
	respondDone(c, "pages.sales.tickets.closed", ticket, err)
}

func (a *SalesController) reopenTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	ticket, err := a.tickets.Reopen(id)
	respondDone(c, "pages.sales.tickets.reopened", ticket, err)
}
