package controller

import (
	"time"

	"github.com/gin-gonic/gin"
)

func (a *ClientController) initGeoRouter(g *gin.RouterGroup) {
	g.GET("/countries", a.connectedCountries)
}

func (a *ClientController) connectedCountries(c *gin.Context) {
	geo, err := a.clientGeo.Snapshot(time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, geo, nil)
}
