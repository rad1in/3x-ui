package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const maxUsageHistoryDays = 90

func (a *ClientController) initUsageRouter(g *gin.RouterGroup) {
	g.GET("/usage/:email", a.usageHistory)
}

func (a *ClientController) usageHistory(c *gin.Context) {
	days := 30
	if raw := c.Query("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		days = min(n, maxUsageHistoryDays)
	}
	rows, err := a.clientService.GetClientUsageHistory(c.Param("email"), days)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}
