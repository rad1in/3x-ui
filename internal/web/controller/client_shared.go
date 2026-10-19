package controller

import "github.com/gin-gonic/gin"

func (a *ClientController) initSharedRouter(g *gin.RouterGroup) {
	g.GET("/sharedAccounts", a.sharedAccounts)
}

func (a *ClientController) sharedAccounts(c *gin.Context) {
	flags, err := a.sharedAccountService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, flags, nil)
}
