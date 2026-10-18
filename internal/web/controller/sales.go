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
	planService  service.PlanService
	orderService service.OrderService
}

func NewSalesController(g *gin.RouterGroup) *SalesController {
	a := &SalesController{}
	g.GET("/plans", a.listPlans)
	g.POST("/plans/save", a.savePlan)
	g.POST("/plans/del/:id", a.deletePlan)
	g.GET("/orders", a.listOrders)
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
