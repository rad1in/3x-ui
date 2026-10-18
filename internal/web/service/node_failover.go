package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

const (
	failoverDefaultAfterSec = 120
	failoverMinAfterSec     = 30
	failoverMaxAfterSec     = 24 * 60 * 60
)

// NodeFailoverTarget is the node subscriptions should advertise instead of n:
// n has failover set, has been offline for its grace period, and the backup is
// an enabled, online node with an address. It returns nil when n should be used.
func NodeFailoverTarget(n *model.Node, byID map[int]*model.Node, now time.Time) *model.Node {
	if n == nil || n.FailoverNodeId <= 0 || n.FailoverNodeId == n.Id {
		return nil
	}
	if n.Status != "offline" || n.LastHeartbeat <= 0 {
		return nil
	}
	if now.Unix()-n.LastHeartbeat < int64(n.FailoverAfterSec) {
		return nil
	}
	backup := byID[n.FailoverNodeId]
	if backup == nil || !backup.Enable || backup.Status != "online" || backup.Address == "" {
		return nil
	}
	return backup
}

// normalizeNodeFailover fills the grace-period default and checks the backup
// exists and is not the node itself.
func normalizeNodeFailover(n *model.Node) error {
	if n.FailoverNodeId < 0 {
		return common.NewError("invalid failover node")
	}
	if n.FailoverAfterSec == 0 {
		n.FailoverAfterSec = failoverDefaultAfterSec
	}
	if n.FailoverAfterSec < failoverMinAfterSec || n.FailoverAfterSec > failoverMaxAfterSec {
		return common.NewErrorf("failover delay must be between %d and %d seconds", failoverMinAfterSec, failoverMaxAfterSec)
	}
	if n.FailoverNodeId == 0 {
		return nil
	}
	if n.Id > 0 && n.FailoverNodeId == n.Id {
		return common.NewError("a node cannot fail over to itself")
	}
	var count int64
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", n.FailoverNodeId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return common.NewError("the failover node does not exist")
	}
	return nil
}
