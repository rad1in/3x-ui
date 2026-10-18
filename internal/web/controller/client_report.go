package controller

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *ClientController) initReportRouter(g *gin.RouterGroup) {
	g.GET("/report", a.usageReport)
}

func (a *ClientController) usageReport(c *gin.Context) {
	days := 30
	if raw := c.Query("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		days = min(n, maxUsageHistoryDays)
	}
	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "html" {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), nil)
		return
	}
	rows, err := a.clientService.UsageReport(requestScope(c), days)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var body bytes.Buffer
	now := time.Now()
	if format == "html" {
		err = service.WriteReportHTML(&body, rows, days, now)
	} else {
		err = service.WriteReportCSV(&body, rows)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if format == "html" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", body.Bytes())
		return
	}
	c.Header("Content-Disposition", `attachment; filename="client-usage-`+now.Format("2006-01-02")+`.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body.Bytes())
}
