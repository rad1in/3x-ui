package controller

import (
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// getRotation returns the inbound's port/SNI rotation setup, or the defaults.
func (a *InboundController) getRotation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	cfg, err := a.rotationService.Get(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, cfg, nil)
}

// saveRotation stores the setup; the schedule starts counting from the save.
func (a *InboundController) saveRotation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var cfg model.InboundRotation
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	cfg.InboundId = id
	saved, err := a.rotationService.Save(&cfg)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), saved, nil)
}

// rotateInbound applies the saved setup now, as a normal inbound update.
func (a *InboundController) rotateInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	res, err := a.rotationService.Rotate(id, time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), res, nil)
	if res.NeedRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.broadcastInboundsUpdate(session.GetLoginUser(c).Id)
	notifyClientsChanged()
}
