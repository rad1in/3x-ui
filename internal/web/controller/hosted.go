package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// hostedForbiddenRoutes would let a hosted panel escape its limits: a restored
// database resets its usage meter, binaries are shared,
// and nesting is unmetered (sponsors are read-only for everyone).
var hostedForbiddenRoutes = map[string]bool{
	"POST /hostedPanels/add":               true,
	"POST /hostedPanels/update/:id":        true,
	"POST /hostedPanels/del/:id":           true,
	"POST /hostedPanels/resetUsage/:id":    true,
	"POST /server/importDB":                true,
	"POST /server/installXray/:version":    true,
	"POST /server/updatePanel":             true,
	"POST /server/updateGeofile":           true,
	"POST /server/updateGeofile/:fileName": true,
	"POST /nodes/updatePanel":              true,
}

// HostedForbiddenRoutes lists the blocked routes for the route contract test.
func HostedForbiddenRoutes() []string {
	out := make([]string, 0, len(hostedForbiddenRoutes))
	for k := range hostedForbiddenRoutes {
		out = append(out, k)
	}
	return out
}

func hostedGuard(c *gin.Context) {
	if hosted.Enabled() && hostedForbiddenRoutes[c.Request.Method+" "+relAPIPath(c.FullPath())] {
		c.AbortWithStatusJSON(http.StatusForbidden, entity.Msg{Success: false, Msg: service.ErrHostedForbidden.Error()})
		return
	}
	c.Next()
}

// HostedController lets a hosted panel show its own limits and usage.
type HostedController struct {
	hostedService service.HostedService
}

func NewHostedController(g *gin.RouterGroup) *HostedController {
	a := &HostedController{}
	g.GET("/status", a.status)
	return a
}

func (a *HostedController) status(c *gin.Context) {
	view, err := a.hostedService.View()
	jsonObj(c, view, err)
}
