package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/wirecodec"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

var approvals = service.NewApprovalService()

// isNodeSyncRequest is true for a master's push to this node, which the master
// already approved; it must never be held for a second admin here.
func isNodeSyncRequest(c *gin.Context) bool {
	scope, _ := c.Get("api_token_scope")
	return scope == model.ApiScopeNodeSync || c.GetHeader(wirecodec.MasterPushHeader) != ""
}

// parkForApproval holds a gated action for a second admin and answers the
// request itself; it reports true when the caller must stop.
func parkForApproval(c *gin.Context, action, target string, payload any) bool {
	actor := session.GetLoginUser(c)
	if actor == nil || isNodeSyncRequest(c) {
		return false
	}
	required, err := approvals.Required(actor, action)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return true
	}
	if !required {
		return false
	}
	row, err := approvals.Request(actor, getRemoteIp(c), action, target, payload)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return true
	}
	jsonMsgObj(c, I18nWeb(c, "pages.approvals.requested"), gin.H{"pendingApproval": true, "id": row.Id}, nil)
	return true
}

// ApprovalController lists pending actions and lets a second admin decide them.
type ApprovalController struct{}

func NewApprovalController(g *gin.RouterGroup) *ApprovalController {
	a := &ApprovalController{}
	g.GET("/list", a.list)
	g.POST("/approve/:id", a.approve)
	g.POST("/reject/:id", a.reject)
	g.POST("/cancel/:id", a.cancel)
	return a
}

func (a *ApprovalController) list(c *gin.Context) {
	rows, err := approvals.List(session.GetLoginUser(c))
	if errors.Is(err, service.ErrApprovalForbidden) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	jsonObj(c, rows, err)
}

func (a *ApprovalController) decide(c *gin.Context, run func(*model.User, string, int) (*model.PendingAction, error), msgKey string) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	row, err := run(session.GetLoginUser(c), getRemoteIp(c), id)
	jsonMsgObj(c, I18nWeb(c, msgKey), row, err)
	if err == nil {
		websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
		websocket.BroadcastInvalidate(websocket.MessageTypeClients)
		websocket.BroadcastInvalidate(websocket.MessageTypeNodes)
	}
}

func (a *ApprovalController) approve(c *gin.Context) {
	a.decide(c, approvals.Approve, "pages.approvals.approved")
}
func (a *ApprovalController) reject(c *gin.Context) {
	a.decide(c, approvals.Reject, "pages.approvals.rejected")
}
func (a *ApprovalController) cancel(c *gin.Context) {
	a.decide(c, approvals.Cancel, "pages.approvals.cancelled")
}
