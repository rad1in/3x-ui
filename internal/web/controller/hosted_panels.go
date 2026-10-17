package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

// HostedPanelController manages the separate panels this server hosts for others.
type HostedPanelController struct {
	hostedPanels panel.HostedPanelService
}

func NewHostedPanelController(g *gin.RouterGroup) *HostedPanelController {
	a := &HostedPanelController{}
	g.GET("/list", a.list)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.POST("/resetUsage/:id", a.resetUsage)
	return a
}

func (a *HostedPanelController) list(c *gin.Context) {
	views, err := a.hostedPanels.List()
	jsonObj(c, views, err)
}

func (a *HostedPanelController) add(c *gin.Context) {
	var in panel.HostedPanelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	created, err := a.hostedPanels.Create(in)
	jsonMsgObj(c, outcomeMsg(c, "pages.hostedPanels.saved", err), created, err)
}

func (a *HostedPanelController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var in panel.HostedPanelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	updated, err := a.hostedPanels.Update(id, in)
	jsonMsgObj(c, outcomeMsg(c, "pages.hostedPanels.saved", err), updated, err)
}

func (a *HostedPanelController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	err = a.hostedPanels.Delete(id)
	jsonMsg(c, outcomeMsg(c, "pages.hostedPanels.deleted", err), err)
}

func (a *HostedPanelController) resetUsage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	err = a.hostedPanels.ResetUsage(id)
	jsonMsg(c, outcomeMsg(c, "pages.hostedPanels.usageReset", err), err)
}

// outcomeMsg keeps a success toast ("Panel saved") off a failed request.
func outcomeMsg(c *gin.Context, successKey string, err error) string {
	if err != nil {
		return I18nWeb(c, "fail")
	}
	return I18nWeb(c, successKey)
}
