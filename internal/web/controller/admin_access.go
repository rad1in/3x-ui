package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

const (
	adminScopeKey = "admin_scope"
	// adminRoleKey is the session admin's role; unset for API tokens, which act as the owner.
	adminRoleKey = "admin_role"
)

// ownerOnlyRoutes manage admins or reach credentials: an admin able to call
// them could make itself an owner (importDB replaces the users table).
var ownerOnlyRoutes = map[string]bool{
	"GET /admins/list":                            true,
	"POST /admins/create":                         true,
	"POST /admins/update/:id":                     true,
	"POST /admins/del/:id":                        true,
	"GET /setting/apiTokens":                      true,
	"POST /setting/apiTokens/create":              true,
	"POST /setting/apiTokens/delete/:id":          true,
	"POST /setting/apiTokens/setEnabled/:id":      true,
	"POST /setting/apiTokens/setAllowedCidrs/:id": true,
	"GET /security/config":                        true,
	"POST /security/config/save":                  true,
	"GET /sales/config":                           true,
	"POST /sales/config/save":                     true,
	"GET /server/getDb":                           true,
	"POST /server/importDB":                       true,
}

// selfServiceRoutes act only on the caller's own account, so every role may use them.
var selfServiceRoutes = map[string]bool{
	"GET /admins/me":                  true,
	"GET /admins/me/dashboardLayout":  true,
	"POST /admins/me/dashboardLayout": true,
	"GET /sessions/list":              true,
	"POST /sessions/revoke/:id":       true,
	"POST /sessions/revokeAll":        true,
	"POST /setting/updateUser":        true,
}

// operatorRule decides one operator request; body is the raw JSON request body.
type operatorRule func(c *gin.Context, scope *service.AdminScope, body []byte) (bool, error)

// operatorRoutes is the whole operator surface. List handlers filter their
// output by the scope; every other rule checks the targets named in the request.
var operatorRoutes = map[string]operatorRule{
	"GET /server/status":               allowInScope,
	"GET /server/getNewUUID":           allowInScope,
	"GET /server/getNewX25519Cert":     allowInScope,
	"GET /server/getNewmldsa65":        allowInScope,
	"GET /server/getNewmlkem768":       allowInScope,
	"GET /server/getNewVlessEnc":       allowInScope,
	"POST /server/getNewEchCert":       allowInScope,
	"POST /setting/defaultSettings":    allowInScope,
	"GET /inbounds/list":               allowInScope,
	"GET /inbounds/list/slim":          allowInScope,
	"GET /inbounds/options":            allowInScope,
	"GET /inbounds/get/:id":            inboundParamInScope,
	"GET /inbounds/:id/openvpnProfile": inboundParamInScope,
	"GET /inbounds/:id/ikev2Profile":   inboundParamInScope,
	"GET /inbounds/:id/sstpCa":         inboundParamInScope,
	"GET /inbounds/:id/ocservCa":       inboundParamInScope,
	"POST /inbounds/update/:id":        inboundParamInScope,
	"POST /inbounds/setEnable/:id":     inboundParamInScope,
	"POST /inbounds/:id/resetTraffic":  inboundParamInScope,
	"POST /inbounds/:id/delAllClients": inboundParamInScope,

	"GET /clients/list":                  allowInScope,
	"GET /clients/list/paged":            allowInScope,
	"POST /clients/onlines":              allowInScope,
	"POST /clients/lastOnline":           allowInScope,
	"POST /clients/renewalPreview":       allowInScope,
	"GET /clients/get/:email":            emailParamInScope,
	"GET /clients/traffic/:email":        emailParamInScope,
	"GET /clients/links/:email":          emailParamInScope,
	"GET /clients/subLinks/:subId":       subIdParamInScope,
	"POST /clients/signedSubLink/:subId": subIdParamInScope,
	"POST /clients/happLink/:id":         clientIdParamInScope,
	"POST /clients/add":                  clientCreateInScope,
	"POST /clients/bulkCreate":           clientBulkCreateInScope,
	"POST /clients/update/:email":        clientUpdateInScope,
	"POST /clients/del/:email":           emailParamInScope,
	"POST /clients/:email/attach":        emailParamAndInboundsInScope,
	"POST /clients/:email/detach":        emailParamAndInboundsInScope,
	"POST /clients/:email/externalLinks": emailParamInScope,
	"POST /clients/resetTraffic/:email":  emailParamInScope,
	"POST /clients/updateTraffic/:email": emailParamInScope,
	"POST /clients/ips/:email":           emailParamInScope,
	"POST /clients/clearIps/:email":      emailParamInScope,
	"POST /clients/hwids/:email":         emailParamInScope,
	"DELETE /clients/hwids/:email":       emailParamInScope,
	"DELETE /clients/hwids/:email/:id":   emailParamInScope,
	"POST /clients/bulkAdjust":           bodyEmailsInScope,
	"POST /clients/bulkEnable":           bodyEmailsInScope,
	"POST /clients/bulkDisable":          bodyEmailsInScope,
	"POST /clients/bulkDel":              bodyEmailsInScope,
	"POST /clients/bulkResetTraffic":     bodyEmailsInScope,
	"POST /clients/bulkAttach":           bodyEmailsAndInboundsInScope,
	"POST /clients/bulkDetach":           bodyEmailsAndInboundsInScope,
	"GET /clients/groups":                allowInScope,
	"GET /clients/groups/:name/emails":   groupParamInScope,
	"POST /clients/groups/resetTraffic":  bodyGroupNameInScope,
	"POST /clients/groups/bulkAdd":       groupBulkAddInScope,
	"POST /clients/groups/bulkRemove":    groupBulkRemoveInScope,
}

// RBACRoutes lists every route the role tables name, for the route contract test.
func RBACRoutes() []string {
	out := make([]string, 0, len(ownerOnlyRoutes)+len(selfServiceRoutes)+len(operatorRoutes))
	for k := range ownerOnlyRoutes {
		out = append(out, k)
	}
	for k := range selfServiceRoutes {
		out = append(out, k)
	}
	for k := range operatorRoutes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// requestScope is the operator scope a handler must filter its output by; nil
// for every other caller.
func requestScope(c *gin.Context) *service.AdminScope {
	if v, ok := c.Get(adminScopeKey); ok {
		scope, _ := v.(*service.AdminScope)
		return scope
	}
	return nil
}

// enforceAdminRole applies the logged-in admin's role to every /panel/api
// route. API tokens and node certificates keep their own scope checks.
func enforceAdminRole(c *gin.Context) {
	if _, viaToken := c.Get("api_token_scope"); viaToken {
		c.Next()
		return
	}
	user := session.GetLoginUser(c)
	if user == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Set(adminRoleKey, user.Role)
	allowed, err := adminMayCall(c, user)
	if err != nil {
		logger.Warning("admin role check:", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, entity.Msg{Success: false, Msg: I18nWeb(c, "somethingWentWrong")})
		return
	}
	if !allowed {
		c.AbortWithStatusJSON(http.StatusForbidden, entity.Msg{Success: false, Msg: I18nWeb(c, "pages.admins.forbidden")})
		return
	}
	c.Next()
}

func adminMayCall(c *gin.Context, user *model.User) (bool, error) {
	key := c.Request.Method + " " + relAPIPath(c.FullPath())
	if selfServiceRoutes[key] {
		return true, nil
	}
	switch user.Role {
	case model.AdminRoleOwner:
		return true, nil
	case model.AdminRoleAdmin:
		return !ownerOnlyRoutes[key], nil
	case model.AdminRoleViewer:
		return !ownerOnlyRoutes[key] && !IsAuditedRoute(c.Request.Method, c.FullPath()), nil
	case model.AdminRoleOperator:
		rule := operatorRoutes[key]
		if rule == nil {
			return false, nil
		}
		scope := service.ScopeForUser(user)
		c.Set(adminScopeKey, scope)
		body, err := peekBody(c)
		if err != nil {
			return false, err
		}
		return rule(c, scope, body)
	default:
		return false, nil
	}
}

// peekBody reads the request body and puts it back for the handler.
func peekBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// decodeBody fills dst from body; a body the handler would reject anyway is
// denied here, so a malformed request can never slip past a check.
func decodeBody(body []byte, dst any) bool {
	return json.Unmarshal(body, dst) == nil
}

func allowInScope(*gin.Context, *service.AdminScope, []byte) (bool, error) { return true, nil }

func inboundParamInScope(c *gin.Context, scope *service.AdminScope, _ []byte) (bool, error) {
	id, err := strconv.Atoi(c.Param("id"))
	return err == nil && scope.HasInbound(id), nil
}

func emailParamInScope(c *gin.Context, scope *service.AdminScope, _ []byte) (bool, error) {
	return scope.HasAllClients([]string{c.Param("email")})
}

func subIdParamInScope(c *gin.Context, scope *service.AdminScope, _ []byte) (bool, error) {
	return scope.HasClientWhere("sub_id = ?", c.Param("subId"))
}

func clientIdParamInScope(c *gin.Context, scope *service.AdminScope, _ []byte) (bool, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return false, nil
	}
	return scope.HasClientWhere("id = ?", id)
}

func emailParamAndInboundsInScope(c *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req attachDetachBody
	if !decodeBody(body, &req) || !scope.HasAllInbounds(req.InboundIds) {
		return false, nil
	}
	return emailParamInScope(c, scope, body)
}

func bodyEmailsInScope(_ *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req struct {
		Emails []string `json:"emails"`
	}
	if !decodeBody(body, &req) {
		return false, nil
	}
	return scope.HasAllClients(req.Emails)
}

func bodyEmailsAndInboundsInScope(c *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req bulkAttachRequest
	if !decodeBody(body, &req) || !scope.HasAllInbounds(req.InboundIds) {
		return false, nil
	}
	return bodyEmailsInScope(c, scope, body)
}

// newClientInScope holds when the operator could see the client it creates:
// every inbound is theirs and the group, if any, is theirs too.
func newClientInScope(scope *service.AdminScope, p service.ClientCreatePayload) bool {
	group := strings.TrimSpace(p.Client.Group)
	if group != "" && !scope.HasGroup(group) {
		return false
	}
	return scope.HasAllInbounds(p.InboundIds) && (len(p.InboundIds) > 0 || group != "")
}

func clientCreateInScope(_ *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var p service.ClientCreatePayload
	return decodeBody(body, &p) && newClientInScope(scope, p), nil
}

func clientBulkCreateInScope(_ *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var payloads []service.ClientCreatePayload
	if !decodeBody(body, &payloads) {
		return false, nil
	}
	for _, p := range payloads {
		if !newClientInScope(scope, p) {
			return false, nil
		}
	}
	return true, nil
}

func clientUpdateInScope(c *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req struct {
		Group string `json:"group"`
	}
	if !decodeBody(body, &req) || !scope.HasAllInbounds(parseInboundIdsQuery(c.Query("inboundIds"))) {
		return false, nil
	}
	return scope.ClientKeepsScope(c.Param("email"), strings.TrimSpace(req.Group))
}

func groupParamInScope(c *gin.Context, scope *service.AdminScope, _ []byte) (bool, error) {
	return scope.HasGroup(c.Param("name")), nil
}

func bodyGroupNameInScope(_ *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req groupResetTrafficBody
	return decodeBody(body, &req) && scope.HasGroup(req.Name), nil
}

func groupBulkAddInScope(c *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req bulkAddToGroupRequest
	if !decodeBody(body, &req) || !scope.HasGroup(req.Group) {
		return false, nil
	}
	return scope.HasAllClients(req.Emails)
}

func groupBulkRemoveInScope(_ *gin.Context, scope *service.AdminScope, body []byte) (bool, error) {
	var req bulkRemoveFromGroupRequest
	if !decodeBody(body, &req) {
		return false, nil
	}
	for _, email := range req.Emails {
		if ok, err := scope.ClientKeepsScope(email, ""); !ok || err != nil {
			return false, err
		}
	}
	return true, nil
}
