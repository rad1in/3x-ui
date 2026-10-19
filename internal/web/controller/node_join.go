package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const (
	joinBodyLimit = 8 << 10
	joinLimit     = 20
	joinWindow    = 10 * time.Minute
)

// NodeJoinController is the one public door for adding a node: it takes no
// session, only a single-use token issued by an admin from the panel.
type NodeJoinController struct {
	joinService service.NodeJoinService
	limiter     *windowLimiter
}

// NewNodeJoinController mounts POST {basePath}/join/node on g.
func NewNodeJoinController(g *gin.RouterGroup) *NodeJoinController {
	a := &NodeJoinController{limiter: newWindowLimiter(joinLimit, joinWindow)}
	g.POST("/join/node", a.join)
	return a
}

func (a *NodeJoinController) join(c *gin.Context) {
	ip := getRemoteIp(c)
	if !a.limiter.allow(ip) {
		c.JSON(http.StatusTooManyRequests, entity.Msg{Msg: "too many attempts, try again later"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, joinBodyLimit)
	var req struct {
		Token string `json:"token"`
		service.NodeJoinRequest
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Msg: "invalid request"})
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		req.Address = ip
	}
	view, err := a.joinService.Join(req.Token, req.NodeJoinRequest)
	if err != nil {
		c.JSON(http.StatusOK, entity.Msg{Msg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: view})
}

// joinCommand issues a join token and the command that installs a node with it.
func (a *NodeController) joinCommand(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		MasterURL string `json:"masterUrl"`
	}
	_ = c.ShouldBindJSON(&req)
	master := strings.TrimSpace(req.MasterURL)
	if master == "" {
		master = requestOrigin(c) + c.GetString("base_path")
	}
	cmd, err := a.joinService.IssueCommand(master, strings.TrimSpace(req.Name))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.add"), err)
		return
	}
	jsonObj(c, cmd, nil)
}

// requestOrigin is scheme://host as the admin's browser reached the panel.
func requestOrigin(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}
