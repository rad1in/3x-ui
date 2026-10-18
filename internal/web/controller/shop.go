package controller

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const (
	shopBodyLimit      = 8 << 10
	shopWriteLimit     = 30
	shopWriteWindow    = 10 * time.Minute
	shopLimiterMaxKeys = 10000
	shopInternalError  = "internal_error"
	shopRateLimited    = "rate_limited"
)

// ShopController is the public side of selling: no panel session, only the
// order token a customer was given. Errors travel as stable codes in msg.
type ShopController struct {
	orderService  service.OrderService
	configService service.SalesConfigService
	limiter       *windowLimiter
	notFound      gin.HandlerFunc
}

// NewShopController mounts the public shop API on g; while the shop is switched
// off every route answers through notFound (the decoy site or a bare 404).
func NewShopController(g *gin.RouterGroup, notFound gin.HandlerFunc) *ShopController {
	a := &ShopController{limiter: newWindowLimiter(shopWriteLimit, shopWriteWindow), notFound: notFound}
	api := g.Group("/api")
	api.Use(a.gate)
	api.GET("/plans", a.catalog)
	api.GET("/orders/:token", a.getOrder)
	api.POST("/orders", a.rateLimited, a.createOrder)
	api.POST("/orders/:token/receipt", a.rateLimited, a.submitReceipt)
	return a
}

func (a *ShopController) gate(c *gin.Context) {
	cfg, err := a.configService.Get()
	if err != nil || !cfg.PublicEnabled {
		a.deny(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Next()
}

func (a *ShopController) deny(c *gin.Context) {
	if a.notFound != nil {
		a.notFound(c)
	} else {
		c.Status(http.StatusNotFound)
	}
	c.Abort()
}

func (a *ShopController) rateLimited(c *gin.Context) {
	if !a.limiter.allow(getRemoteIp(c)) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, entity.Msg{Msg: shopRateLimited})
		return
	}
	c.Next()
}

// reply sends obj, or err as its code when it is a customer-safe refusal; any
// other error is logged and hidden.
func (a *ShopController) reply(c *gin.Context, obj any, err error) {
	var salesErr *service.SalesError
	switch {
	case err == nil:
		c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: obj})
	case errors.As(err, &salesErr):
		c.JSON(http.StatusOK, entity.Msg{Msg: salesErr.Code})
	default:
		logger.Warningf("shop %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, entity.Msg{Msg: shopInternalError})
	}
}

func (a *ShopController) bind(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, shopBodyLimit)
	if err := c.ShouldBindJSON(dst); err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Msg: "bad_request"})
		return false
	}
	return true
}

func (a *ShopController) catalog(c *gin.Context) {
	catalog, err := a.orderService.Catalog()
	a.reply(c, catalog, err)
}

func (a *ShopController) view(c *gin.Context, token string) {
	order, err := a.orderService.GetOrderByToken(token)
	if err != nil {
		a.reply(c, nil, err)
		return
	}
	view, err := a.orderService.PublicView(order, resolveHost(c))
	a.reply(c, view, err)
}

func (a *ShopController) getOrder(c *gin.Context) {
	a.view(c, c.Param("token"))
}

func (a *ShopController) createOrder(c *gin.Context) {
	var in service.CreateOrderInput
	if !a.bind(c, &in) {
		return
	}
	order, err := a.orderService.CreateOrder(in)
	if err != nil {
		a.reply(c, nil, err)
		return
	}
	a.view(c, order.Token)
}

type shopReceiptBody struct {
	Ref  string `json:"ref"`
	Note string `json:"note"`
}

func (a *ShopController) submitReceipt(c *gin.Context) {
	var body shopReceiptBody
	if !a.bind(c, &body) {
		return
	}
	order, err := a.orderService.SubmitReceipt(c.Param("token"), body.Ref, body.Note)
	if err != nil {
		a.reply(c, nil, err)
		return
	}
	view, err := a.orderService.PublicView(order, resolveHost(c))
	a.reply(c, view, err)
}

// windowLimiter allows limit calls per key in each fixed window.
type windowLimiter struct {
	mu     sync.Mutex
	now    func() time.Time
	limit  int
	window time.Duration
	hits   map[string]*windowHits
}

type windowHits struct {
	start time.Time
	count int
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{now: time.Now, limit: limit, window: window, hits: make(map[string]*windowHits)}
}

func (l *windowLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) >= shopLimiterMaxKeys {
		for k, h := range l.hits {
			if now.Sub(h.start) >= l.window {
				delete(l.hits, k)
			}
		}
		for k := range l.hits {
			if len(l.hits) < shopLimiterMaxKeys {
				break
			}
			delete(l.hits, k)
		}
	}
	h := l.hits[key]
	if h == nil || now.Sub(h.start) >= l.window {
		l.hits[key] = &windowHits{start: now, count: 1}
		return true
	}
	h.count++
	return h.count <= l.limit
}
