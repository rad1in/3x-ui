package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// ResellerController serves the credit and sales of resellers: the signed-in
// reseller reads its own, owners manage everyone's.
type ResellerController struct {
	reseller service.ResellerService
}

func NewResellerController(g *gin.RouterGroup) *ResellerController {
	a := &ResellerController{}
	g.GET("/me", a.me)
	g.GET("/report", a.report)
	g.GET("/account/:id", a.account)
	g.POST("/topup/:id", a.topup)
	g.POST("/prices/:id", a.setPrices)
	g.GET("/config", a.getConfig)
	g.POST("/config/save", a.saveConfig)
	return a
}

func (a *ResellerController) getConfig(c *gin.Context) {
	cfg, err := a.reseller.Config()
	jsonObj(c, cfg, err)
}

func (a *ResellerController) saveConfig(c *gin.Context) {
	var cfg service.ResellerConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.reseller.toasts.configSaved", nil, a.reseller.SaveConfig(cfg))
}

func (a *ResellerController) me(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil || user.Role != model.AdminRoleOperator {
		jsonObj(c, &service.ResellerAccount{Entries: []model.ResellerEntry{}}, nil)
		return
	}
	acc, err := a.reseller.Account(user)
	jsonObj(c, acc, err)
}

func (a *ResellerController) account(c *gin.Context) {
	user, ok := a.resellerParam(c)
	if !ok {
		return
	}
	acc, err := a.reseller.Account(user)
	jsonObj(c, acc, err)
}

// report is every reseller's sales for an owner or admin and only the caller's
// own for a reseller.
func (a *ResellerController) report(c *gin.Context) {
	from, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	to, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	var only *int
	if user := session.GetLoginUser(c); user != nil && user.Role == model.AdminRoleOperator {
		only = &user.Id
	}
	rep, err := a.reseller.Report(from, to, only)
	jsonObj(c, rep, err)
}

type resellerTopupBody struct {
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
}

func (a *ResellerController) topup(c *gin.Context) {
	user, ok := a.resellerParam(c)
	if !ok {
		return
	}
	var body resellerTopupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	entry, err := a.reseller.Topup(user.Id, body.Amount, body.Note)
	respondDone(c, "pages.reseller.toasts.toppedUp", entry, err)
}

type resellerPricesBody struct {
	PricePerGB  *int64 `json:"pricePerGb"`
	PricePerDay *int64 `json:"pricePerDay"`
}

func (a *ResellerController) setPrices(c *gin.Context) {
	user, ok := a.resellerParam(c)
	if !ok {
		return
	}
	var body resellerPricesBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	respondDone(c, "pages.reseller.toasts.pricesSaved", nil, a.reseller.SetPrices(user.Id, body.PricePerGB, body.PricePerDay))
}

// resellerParam loads the reseller named by :id, answering the failure itself.
func (a *ResellerController) resellerParam(c *gin.Context) (*model.User, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return nil, false
	}
	var user model.User
	if err := database.GetDB().Where("id = ? AND role = ?", id, model.AdminRoleOperator).First(&user).Error; err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), service.ErrResellerNotFound)
		return nil, false
	}
	return &user, true
}
