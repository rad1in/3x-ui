package controller

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// metricsStaleAfter is how old the cached host status may be before a scrape
// refreshes it; the 2s ticker is not running in every process.
const metricsStaleAfter = 10 * time.Second

// MetricsController serves the opt-in Prometheus endpoint at <basePath>metrics.
type MetricsController struct {
	settings service.SettingService
	server   service.ServerService
	metrics  service.MetricsService
}

func NewMetricsController(g *gin.RouterGroup) *MetricsController {
	a := &MetricsController{}
	g.GET("/metrics", a.serve)
	return a
}

// scrapeToken reads the credential from "Bearer <token>" or from the password
// of Basic auth (any username), the two forms Prometheus can send.
func scrapeToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	if _, pass, ok := c.Request.BasicAuth(); ok {
		return pass
	}
	return ""
}

func (a *MetricsController) serve(c *gin.Context) {
	enabled, err := a.settings.GetMetricsEnable()
	token, tokenErr := a.settings.GetMetricsToken()
	token = strings.TrimSpace(token)
	// A disabled or token-less endpoint looks absent, never open or probeable.
	if err != nil || tokenErr != nil || !enabled || token == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(scrapeToken(c)), []byte(token)) != 1 {
		c.Header("WWW-Authenticate", `Basic realm="eternal metrics"`)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	status := a.server.CurrentStatus()
	if status == nil || time.Since(status.T) > metricsStaleAfter {
		status = a.server.RefreshStatus()
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(a.metrics.Render(status)))
}
