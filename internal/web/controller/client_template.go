package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"github.com/gin-gonic/gin"
)

func (a *ClientController) initTemplateRouter(g *gin.RouterGroup) {
	g.GET("/templates", a.listTemplates)
	g.POST("/templates/save", a.saveTemplate)
	g.POST("/templates/del/:id", a.deleteTemplate)
	g.POST("/templates/apply", a.applyTemplate)
}

func (a *ClientController) listTemplates(c *gin.Context) {
	rows, err := a.clientService.ListClientTemplates()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ClientController) saveTemplate(c *gin.Context) {
	var tpl model.ClientTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	saved, err := a.clientService.SaveClientTemplate(&tpl)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, saved, nil)
}

func (a *ClientController) deleteTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.clientService.DeleteClientTemplate(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.clients.templates.deleted"), nil)
}

type applyTemplateBody struct {
	TemplateId int      `json:"templateId"`
	Emails     []string `json:"emails"`
}

func (a *ClientController) applyTemplate(c *gin.Context) {
	var body applyTemplateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	res, err := a.clientService.CreateClientsFromTemplate(&a.inboundService, body.TemplateId, body.Emails)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if res.NeedRestart {
		a.xrayService.SetToNeedRestart()
	}
	if res.NeedRestart || len(res.Created) > 0 {
		notifyClientsChanged()
	}
	jsonObj(c, res, nil)
}
