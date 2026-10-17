package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

// SponsorAdminController lets the panel owner manage their own sponsor slots.
type SponsorAdminController struct {
	panelService panel.PanelService
}

func NewSponsorAdminController(g *gin.RouterGroup) *SponsorAdminController {
	a := &SponsorAdminController{}
	g.GET("/list", a.list)
	g.POST("/save", a.save)
	g.POST("/del/:id", a.del)
	g.POST("/config", a.config)
	return a
}

func (a *SponsorAdminController) list(c *gin.Context) {
	view, err := a.panelService.SponsorAdmin()
	jsonObj(c, view, err)
}

func (a *SponsorAdminController) save(c *gin.Context) {
	var rec model.SponsorRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	saved, err := a.panelService.SaveSponsorRecord(rec)
	jsonMsgObj(c, I18nWeb(c, "pages.sponsors.admin.saved"), saved, err)
}

func (a *SponsorAdminController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.sponsors.admin.deleted"), a.panelService.DeleteSponsorRecord(id))
}

func (a *SponsorAdminController) config(c *gin.Context) {
	var cfg panel.SponsorConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.sponsors.admin.saved"), a.panelService.SetSponsorConfig(cfg))
}
