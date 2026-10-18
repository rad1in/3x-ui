package controller

import "github.com/gin-gonic/gin"

func (a *ClientController) initDisconnectRouter(g *gin.RouterGroup) {
	g.POST("/disconnect/:email", a.disconnect)
}

func (a *ClientController) disconnect(c *gin.Context) {
	res, err := a.clientService.DisconnectClient(&a.inboundService, c.Param("email"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, res, nil)
}
