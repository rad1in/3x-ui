package service

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	capStateReached = "reached"
	capActionAlert  = "alert"
	capActionStop   = "disable"
	maxBillingDay   = 28
)

// NodeCapHit is a node whose monthly cap was just reached.
type NodeCapHit struct {
	NodeId    int
	Name      string
	UsedBytes int64
	CapBytes  int64
	Action    string
	// Stopped is true when the node's inbounds were disabled as well.
	Stopped bool
}

// NodeCapService meters the traffic the panel accounts on each node's inbounds
// per billing period and applies the node's cap action when it is reached.
type NodeCapService struct {
	inbounds InboundService
}

// capPeriodStart is the start of the billing period containing now. Billing
// days above 28 are clamped so every month has one; 0 means the 1st.
func capPeriodStart(now time.Time, billingDay int) time.Time {
	day := min(max(billingDay, 1), maxBillingDay)
	start := time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, now.Location())
	if start.After(now) {
		start = time.Date(now.Year(), now.Month()-1, day, 0, 0, 0, 0, now.Location())
	}
	return start
}

func nodeCapBytes(n *model.Node) int64 {
	if n.MonthlyCapGB <= 0 {
		return 0
	}
	return int64(n.MonthlyCapGB * float64(1<<30))
}

// Tick advances every node's period usage and returns the nodes that reached
// their cap on this tick. Usage grows by the increase of the node's inbound
// counters, so a counter reset or a deleted inbound never counts as traffic.
func (s *NodeCapService) Tick(now time.Time) []NodeCapHit {
	db := database.GetDB()
	var nodes []model.Node
	if err := db.Find(&nodes).Error; err != nil {
		logger.Warning("node cap: read nodes failed:", err)
		return nil
	}
	var rows []struct {
		NodeId int
		Total  int64
	}
	if err := db.Table("inbounds").
		Select("node_id, COALESCE(SUM(up + down), 0) AS total").
		Where("node_id IS NOT NULL").
		Group("node_id").
		Scan(&rows).Error; err != nil {
		logger.Warning("node cap: sum inbound traffic failed:", err)
		return nil
	}
	totals := make(map[int]int64, len(rows))
	for _, r := range rows {
		totals[r.NodeId] = r.Total
	}

	var hits []NodeCapHit
	for i := range nodes {
		if hit := s.tickNode(&nodes[i], totals[nodes[i].Id], now); hit != nil {
			hits = append(hits, *hit)
		}
	}
	return hits
}

func (s *NodeCapService) tickNode(n *model.Node, total int64, now time.Time) *NodeCapHit {
	updates := map[string]any{}
	periodStart := capPeriodStart(now, n.BillingDay).Unix()
	used, state, disabled := n.PeriodBytes, n.CapState, n.CapDisabledInbounds

	if !n.CapPrimed {
		updates["cap_primed"] = true
	} else if total >= n.CapLastTotal {
		used += total - n.CapLastTotal
	}
	updates["cap_last_total"] = total

	if n.PeriodStart != periodStart {
		used, state = 0, ""
		disabled = s.restoreInbounds(disabled)
		updates["period_start"] = periodStart
	}

	capBytes := nodeCapBytes(n)
	reached := capBytes > 0 && used >= capBytes
	if state == capStateReached && !reached {
		// The cap was raised or removed mid-period.
		state = ""
		disabled = s.restoreInbounds(disabled)
	}

	var hit *NodeCapHit
	if reached && state != capStateReached {
		state = capStateReached
		hit = &NodeCapHit{NodeId: n.Id, Name: n.Name, UsedBytes: used, CapBytes: capBytes, Action: n.CapAction}
		if n.CapAction == capActionStop {
			disabled = s.disableInbounds(n.Id)
			hit.Stopped = true
		}
	}

	updates["period_bytes"] = used
	updates["cap_state"] = state
	updates["cap_disabled_inbounds"] = disabledJSON(disabled)
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", n.Id).Updates(updates).Error; err != nil {
		logger.Warning("node cap: save node", n.Id, "failed:", err)
		return nil
	}
	return hit
}

func disabledJSON(ids []int) string {
	if ids == nil {
		ids = []int{}
	}
	raw, _ := json.Marshal(ids)
	return string(raw)
}

// disableInbounds stops the node's enabled inbounds through the normal enable
// path (so a remote node is pushed or marked dirty) and returns the ones it stopped.
func (s *NodeCapService) disableInbounds(nodeID int) []int {
	var ids []int
	if err := database.GetDB().Model(&model.Inbound{}).
		Where("node_id = ? AND enable = ?", nodeID, true).
		Pluck("id", &ids).Error; err != nil {
		logger.Warning("node cap: list inbounds of node", nodeID, "failed:", err)
		return nil
	}
	stopped := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, err := s.inbounds.SetInboundEnable(id, false); err != nil {
			logger.Warning("node cap: disable inbound", id, "failed:", err)
			continue
		}
		stopped = append(stopped, id)
	}
	slices.Sort(stopped)
	return stopped
}

// restoreInbounds re-enables the inbounds the cap stopped; an inbound the admin
// has since deleted is simply gone. It returns the ids it could not restore.
func (s *NodeCapService) restoreInbounds(ids []int) []int {
	var left []int
	for _, id := range ids {
		if _, err := s.inbounds.SetInboundEnable(id, true); err != nil && !database.IsNotFound(err) {
			logger.Warning("node cap: re-enable inbound", id, "failed:", err)
			left = append(left, id)
		}
	}
	return left
}
