package controller

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

// A year is the longest session max-age the settings allow, so older rows are dead.
const sessionPruneAfter = 366 * 24 * time.Hour

func init() { session.ClientIP = getRemoteIp }

// SessionController lists signed-in browsers and ends them. The role guard lets
// every role in; the service restricts non-owners to their own sessions.
type SessionController struct {
	sessionService panel.SessionService
	settingService service.SettingService
}

func NewSessionController(g *gin.RouterGroup) *SessionController {
	a := &SessionController{}
	g.GET("/list", a.list)
	g.POST("/revoke/:id", a.revoke)
	g.POST("/revokeAll", a.revokeAll)
	return a
}

type sessionListQuery struct {
	UserId int `form:"userId"`
}

type revokeAllBody struct {
	UserId      int  `json:"userId"`
	KeepCurrent bool `json:"keepCurrent"`
}

// idleLimit is how long a session may sit unused before it stops counting as active.
func (a *SessionController) idleLimit() time.Duration {
	minutes, err := a.settingService.GetSessionMaxAge()
	if err != nil || minutes <= 0 {
		minutes = 360
	}
	return time.Duration(minutes) * time.Minute
}

func (a *SessionController) list(c *gin.Context) {
	actor := session.GetLoginUser(c)
	var q sessionListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	rows, err := a.sessionService.List(actor, q.UserId, session.CurrentSessionId(c), a.idleLimit())
	jsonObj(c, rows, err)
}

func (a *SessionController) revoke(c *gin.Context) {
	userId, err := a.sessionService.Revoke(session.GetLoginUser(c), c.Param("id"))
	if err == nil && userId != 0 {
		websocket.DisconnectUser(userId)
	}
	jsonMsg(c, I18nWeb(c, "pages.sessions.revoked"), err)
}

func (a *SessionController) revokeAll(c *gin.Context) {
	actor := session.GetLoginUser(c)
	var body revokeAllBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	keep := ""
	if body.KeepCurrent {
		keep = session.CurrentSessionId(c)
	}
	n, err := a.sessionService.RevokeAll(actor, body.UserId, keep)
	if err == nil && keep == "" && (body.UserId == 0 || body.UserId == actor.Id) {
		_ = session.ClearSession(c)
	}
	if err == nil {
		target := body.UserId
		if target == 0 {
			target = actor.Id
		}
		// Sockets are per user, not per session; the spared browser reconnects by itself.
		websocket.DisconnectUser(target)
	}
	jsonMsgObj(c, I18nWeb(c, "pages.sessions.revokedAll"), gin.H{"revoked": n}, err)
}
