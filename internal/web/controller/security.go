package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// SecurityController serves the owner-only extra protections.
type SecurityController struct {
	config service.SecurityConfigService
}

func NewSecurityController(g *gin.RouterGroup) *SecurityController {
	a := &SecurityController{}
	g.GET("/config", a.getConfig)
	g.POST("/config/save", a.saveConfig)
	return a
}

func (a *SecurityController) getConfig(c *gin.Context) {
	view, err := a.config.GetView()
	jsonObj(c, view, err)
}

func (a *SecurityController) saveConfig(c *gin.Context) {
	var cfg service.SecurityConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.settings.security.extra.saved", nil, a.config.Save(&cfg))
}
