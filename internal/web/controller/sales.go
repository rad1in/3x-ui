package controller

import (
	"strconv"

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
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewSalesController(g *gin.RouterGroup) *SalesController {
	a := &SalesController{}
	g.GET("/plans", a.listPlans)
	g.POST("/plans/save", a.savePlan)
	g.POST("/plans/del/:id", a.deletePlan)
	g.GET("/orders", a.listOrders)
	g.POST("/orders/approve/:id", a.approveOrder)
	g.POST("/orders/reject/:id", a.rejectOrder)
	g.GET("/config", a.getConfig)
	g.POST("/config/save", a.saveConfig)
	return a
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
	jsonMsgObj(c, I18nWeb(c, "pages.sales.plans.saved"), saved, err)
}

func (a *SalesController) deletePlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.sales.plans.deleted"), a.planService.DeletePlan(id))
}

func (a *SalesController) listOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	page, err := a.orderService.ListOrders(c.Query("status"), limit, offset)
	jsonObj(c, page, err)
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
	jsonMsgObj(c, I18nWeb(c, "pages.sales.orders.approved"), order, err)
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
	jsonMsgObj(c, I18nWeb(c, "pages.sales.orders.rejected"), order, err)
}

func (a *SalesController) getConfig(c *gin.Context) {
	cfg, err := a.configService.Get()
	jsonObj(c, cfg, err)
}

func (a *SalesController) saveConfig(c *gin.Context) {
	var cfg service.SalesConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.sales.config.saved"), a.configService.Save(&cfg))
}
