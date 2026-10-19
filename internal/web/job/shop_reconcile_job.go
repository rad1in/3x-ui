package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"
)

// ShopReconcileJob settles online-payment orders whose customer never came
// back from the gateway, and abandons the ones nobody paid.
type ShopReconcileJob struct {
	orders         service.OrderService
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewShopReconcileJob() *ShopReconcileJob {
	return new(ShopReconcileJob)
}

func (j *ShopReconcileJob) Run() {
	fulfilled, needRestart, err := j.orders.ReconcileOnline(&j.inboundService)
	if err != nil {
		logger.Warning("shop reconcile:", err)
	}
	if fulfilled > 0 {
		logger.Infof("shop reconcile: delivered %d paid order(s)", fulfilled)
		websocket.BroadcastInvalidate(websocket.MessageTypeClients)
	}
	if needRestart {
		j.xrayService.SetToNeedRestart()
	}
}
