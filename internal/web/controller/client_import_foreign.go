package controller

import "github.com/gin-gonic/gin"

func (a *ClientController) initForeignImportRouter(g *gin.RouterGroup) {
	g.POST("/importForeign", a.importForeign)
}

type importForeignRequest struct {
	Source     string `json:"source"`
	Data       string `json:"data"`
	InboundIds []int  `json:"inboundIds"`
}

func (a *ClientController) importForeign(c *gin.Context) {
	var req importForeignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	result, needRestart, err := a.clientService.ImportForeignClients(&a.inboundService, req.Source, []byte(req.Data), req.InboundIds)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if needRestart || result.Created > 0 {
		notifyClientsChanged()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
}
