package controller

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

const (
	auditSuccessKey = "audit_success"
	apiTokenNameKey = "api_token_name"
)

// auditSkipRoutes lists the non-GET API routes that only read or probe. Every
// other non-GET route is audited; TestAuditRouteClassification pins both ways.
var auditSkipRoutes = map[string]bool{
	"POST /inbounds/pushClientTraffics": true,
	"POST /server/logs/:count":          true,
	"POST /server/xraylogs/:count":      true,
	"POST /server/amneziawglogs/:count": true,
	"POST /server/getNewEchCert":        true,
	"POST /server/getCertHash":          true,
	"POST /server/getRemoteCertHash":    true,
	"POST /server/scanRealityTarget":    true,
	"POST /server/scanRealityTargets":   true,
	"POST /server/clientIps":            true,
	"POST /clients/renewalPreview":      true,
	"POST /clients/ips/:email":          true,
	"POST /clients/hwids/:email":        true,
	"POST /clients/onlines":             true,
	"POST /clients/onlinesByGuid":       true,
	"POST /clients/clientIpsByGuid":     true,
	"POST /clients/activeInbounds":      true,
	"POST /clients/lastOnline":          true,
	"POST /clients/happLink/:id":        true,
	"POST /nodes/test":                  true,
	"POST /nodes/certFingerprint":       true,
	"POST /nodes/inbounds":              true,
	"POST /nodes/probe/:id":             true,
	"POST /setting/all":                 true,
	"POST /setting/defaultSettings":     true,
	"POST /setting/factoryDefaults":     true,
	"POST /setting/validateRegex":       true,
	"POST /setting/testSmtp":            true,
	"POST /setting/testTgBot":           true,
	"POST /setting/testDiscord":         true,
	"POST /setting/testWebhook":         true,
	"POST /setting/testPush":            true,
	"POST /xray/":                       true,
	"POST /xray/testOutbound":           true,
	"POST /xray/testOutbounds":          true,
	"POST /xray/balancerStatus":         true,
	"POST /xray/routeTest":              true,
	"POST /xray/geodata/validate":       true,
	"POST /xray/outbound-subs/parse":    true,
}

// IsAuditedRoute reports whether a request on fullPath (a gin route template)
// lands in the activity log.
func IsAuditedRoute(method, fullPath string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return !auditSkipRoutes[method+" "+relAPIPath(fullPath)]
}

// AuditSkippedRoutes returns the skip list, sorted, for the route contract test.
func AuditSkippedRoutes() []string {
	out := make([]string, 0, len(auditSkipRoutes))
	for k := range auditSkipRoutes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// auditMiddleware records each audited API call after its handler ran. Request
// bodies are never stored: they can carry passwords, keys and tokens.
func auditMiddleware(audit *service.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		fullPath := c.FullPath()
		if fullPath == "" || !IsAuditedRoute(c.Request.Method, fullPath) {
			return
		}
		status := c.Writer.Status()
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return
		}
		success := status < http.StatusBadRequest
		if v, ok := c.Get(auditSuccessKey); ok {
			success, _ = v.(bool)
		}
		err := audit.Record(model.AuditLog{
			CreatedAt: time.Now().UnixMilli(),
			Actor:     auditActor(c),
			Ip:        getRemoteIp(c),
			Action:    c.Request.Method + " " + relAPIPath(fullPath),
			Target:    auditTarget(c.Params),
			Success:   success,
		})
		if err != nil {
			logger.Warning("audit: record", fullPath, err)
		}
	}
}

func auditActor(c *gin.Context) string {
	if name := c.GetString(apiTokenNameKey); name != "" {
		return "token:" + name
	}
	if c.Request.TLS != nil && len(c.Request.TLS.VerifiedChains) > 0 {
		return "node:mtls"
	}
	if u := session.GetLoginUser(c); u != nil {
		return u.Username
	}
	return ""
}

func auditTarget(params gin.Params) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, p.Key+"="+p.Value)
	}
	return strings.Join(parts, " ")
}

// AuditController serves the activity log.
type AuditController struct {
	auditService service.AuditService
}

func NewAuditController(g *gin.RouterGroup) *AuditController {
	a := &AuditController{}
	g.GET("/list", a.list)
	return a
}

func (a *AuditController) list(c *gin.Context) {
	var params service.AuditListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	page, err := a.auditService.List(params)
	jsonObj(c, page, err)
}
