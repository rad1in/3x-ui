package controller

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"

	"github.com/gin-gonic/gin"
)

const (
	shopBodyLimit      = 8 << 10
	shopIPNBodyLimit   = 64 << 10
	shopWriteLimit     = 30
	shopWriteWindow    = 10 * time.Minute
	shopLimiterMaxKeys = 10000
	shopInternalError  = "internal_error"
	shopRateLimited    = "rate_limited"
)

// ShopController is the public side of selling: no panel session, only the
// order token a customer was given. Errors travel as stable codes in msg.
type ShopController struct {
	orderService   *service.OrderService
	configService  service.SalesConfigService
	inboundService service.InboundService
	xrayService    service.XrayService
	limiter        *windowLimiter
	notFound       gin.HandlerFunc
}

// NewShopController mounts the public shop API on g; while the shop is switched
// off every route answers through notFound (the decoy site or a bare 404).
func NewShopController(g *gin.RouterGroup, notFound gin.HandlerFunc) *ShopController {
	return newShopController(g, notFound, &service.OrderService{})
}

func newShopController(g *gin.RouterGroup, notFound gin.HandlerFunc, orders *service.OrderService) *ShopController {
	a := &ShopController{orderService: orders, limiter: newWindowLimiter(shopWriteLimit, shopWriteWindow), notFound: notFound}
	g.GET("/", a.gate, a.page)
	api := g.Group("/api")
	api.Use(a.gate)
	api.GET("/plans", a.catalog)
	api.GET("/orders/:token", a.getOrder)
	api.POST("/orders", a.rateLimited, a.createOrder)
	api.POST("/orders/:token/receipt", a.rateLimited, a.submitReceipt)
	api.POST("/quote", a.rateLimited, a.quote)
	api.GET("/wallet", a.rateLimited, a.wallet)
	api.POST("/ipn/nowpayments", a.ipn)
	api.POST("/webhook/stripe", a.stripeWebhook)
	api.GET("/callback/:gateway", a.rateLimited, a.callback)
	api.POST("/callback/:gateway", a.rateLimited, a.callback)
	return a
}

// page serves the shop bundle. Its assets are rewritten to /shop/assets and
// the page is told that base path, so nothing in it names the panel's own
// secret base path.
func (a *ShopController) page(c *gin.Context) {
	body, err := fs.ReadFile(distFS, "dist/shop.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "missing embedded page: shop.html")
		return
	}
	body = bytes.ReplaceAll(body, []byte(`="/assets/`), []byte(`="/shop/assets/`))
	inject := `<script data-cfasync="false">window.X_UI_BASE_PATH="/shop/"</script></head>`
	body = bytes.Replace(body, []byte("</head>"), []byte(inject), 1)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
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
	order, needRestart, err := a.orderService.Checkout(&a.inboundService, in)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if err != nil {
		a.reply(c, nil, err)
		return
	}
	if order.FulfilledAt > 0 {
		notifyClientsChanged()
	}
	a.view(c, order.Token)
}

// callback is where a payment gateway sends the customer back. The redirect
// only says whether to look; the order is settled by asking the gateway.
func (a *ShopController) callback(c *gin.Context) {
	gateway, token := c.Param("gateway"), c.Query("order")
	_ = c.Request.ParseForm()
	var claimsPaid bool
	switch gateway {
	case service.GatewayZarinpal:
		claimsPaid = payment.ZarinpalCallbackPaid(c.Request.URL.Query())
	case service.GatewayIDPay:
		claimsPaid = payment.IDPayCallbackPaid(c.Request.Form)
	case service.GatewayStripe, service.GatewayPayPal:
		claimsPaid = payment.CheckoutReturnPaid(c.Request.URL.Query())
	default:
		a.deny(c)
		return
	}
	order, needRestart, err := a.orderService.CompleteGatewayPayment(&a.inboundService, token, claimsPaid)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if order != nil && order.FulfilledAt > 0 {
		notifyClientsChanged()
	}
	var salesErr *service.SalesError
	known := errors.As(err, &salesErr)
	if err != nil && !known && !errors.Is(err, payment.ErrGateway) {
		logger.Warningf("shop callback %s: %v", gateway, err)
	}
	if known && salesErr.Code == service.ErrOrderNotFound {
		c.Status(http.StatusNotFound)
		return
	}
	cfg, cfgErr := a.configService.Get()
	if cfgErr != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Redirect(http.StatusSeeOther, service.ShopOrderURL(cfg, token))
}

// ipn receives NOWPayments webhooks. A forged one is refused before anything
// else; a verified one the shop cannot use is answered 200 so the sender stops
// retrying, while a failure on our side answers 500 so it tries again.
func (a *ShopController) ipn(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, shopIPNBodyLimit)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Msg: "bad_request"})
		return
	}
	order, needRestart, err := a.orderService.HandleCryptoIPN(&a.inboundService, body, c.GetHeader("x-nowpayments-sig"))
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if order != nil && order.FulfilledAt > 0 {
		notifyClientsChanged()
	}
	var salesErr *service.SalesError
	switch {
	case err == nil:
		c.JSON(http.StatusOK, entity.Msg{Success: true})
	case errors.As(err, &salesErr) && salesErr.Code == service.ErrBadSignature:
		c.JSON(http.StatusUnauthorized, entity.Msg{Msg: salesErr.Code})
	case errors.As(err, &salesErr) && salesErr.Code == service.ErrGatewayUnavailable:
		a.deny(c)
	case errors.As(err, &salesErr), errors.Is(err, payment.ErrGateway):
		c.JSON(http.StatusOK, entity.Msg{Msg: err.Error()})
	default:
		logger.Warningf("shop crypto ipn: %v", err)
		c.JSON(http.StatusInternalServerError, entity.Msg{Msg: shopInternalError})
	}
}

// stripeWebhook receives Stripe Checkout events. A forged one is refused; an
// event that needs no action is answered 200, while a failure on our side or
// at Stripe answers 500 so Stripe delivers it again.
func (a *ShopController) stripeWebhook(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, shopIPNBodyLimit)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Msg: "bad_request"})
		return
	}
	order, needRestart, err := a.orderService.HandleStripeWebhook(&a.inboundService, body, c.GetHeader("Stripe-Signature"))
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if order != nil && order.FulfilledAt > 0 {
		notifyClientsChanged()
	}
	var salesErr *service.SalesError
	switch {
	case err == nil:
		c.JSON(http.StatusOK, entity.Msg{Success: true})
	case errors.As(err, &salesErr) && salesErr.Code == service.ErrBadSignature:
		c.JSON(http.StatusUnauthorized, entity.Msg{Msg: salesErr.Code})
	case errors.As(err, &salesErr) && salesErr.Code == service.ErrGatewayUnavailable:
		a.deny(c)
	case errors.As(err, &salesErr):
		c.JSON(http.StatusOK, entity.Msg{Msg: salesErr.Code})
	default:
		logger.Warningf("shop stripe webhook: %v", err)
		c.JSON(http.StatusInternalServerError, entity.Msg{Msg: shopInternalError})
	}
}

func (a *ShopController) quote(c *gin.Context) {
	var in service.QuoteInput
	if !a.bind(c, &in) {
		return
	}
	quote, err := a.orderService.Quote(in)
	a.reply(c, quote, err)
}

func (a *ShopController) wallet(c *gin.Context) {
	view, err := a.orderService.CustomerWallet(c.Query("email"), c.Query("proof"))
	a.reply(c, view, err)
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
