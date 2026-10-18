package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

// AdminController lets owners manage the panel's admin accounts.
type AdminController struct {
	userService panel.UserService
}

func NewAdminController(g *gin.RouterGroup) *AdminController {
	a := &AdminController{}
	g.GET("/me", a.me)
	g.GET("/list", a.list)
	g.POST("/create", a.create)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	return a
}

// me tells the UI who is signed in and with which role; an API token acts as
// the primary owner.
func (a *AdminController) me(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), panel.ErrAdminNotFound)
		return
	}
	jsonObj(c, panel.ToAdminView(user), nil)
}

func (a *AdminController) list(c *gin.Context) {
	admins, err := a.userService.ListAdmins()
	jsonObj(c, admins, err)
}

func (a *AdminController) create(c *gin.Context) {
	var in panel.AdminInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	view, err := a.userService.CreateAdmin(in)
	jsonMsgObj(c, I18nWeb(c, "pages.admins.toasts.saved"), view, err)
}

func (a *AdminController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var in panel.AdminInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	view, err := a.userService.UpdateAdmin(actorId(c), id, in)
	if err == nil {
		websocket.DisconnectUser(id)
	}
	jsonMsgObj(c, I18nWeb(c, "pages.admins.toasts.saved"), view, err)
}

func (a *AdminController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	err = a.userService.DeleteAdmin(actorId(c), id)
	if err == nil {
		websocket.DisconnectUser(id)
	}
	jsonMsg(c, I18nWeb(c, "pages.admins.toasts.deleted"), err)
}

func actorId(c *gin.Context) int {
	if u := session.GetLoginUser(c); u != nil {
		return u.Id
	}
	return 0
}
