package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *ClientController) transfer(c *gin.Context) {
	var req service.ClientTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	res, needRestart, err := a.transferService.Transfer(&a.inboundService, req)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if needRestart || err == nil {
		notifyClientsChanged()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, res, nil)
}
