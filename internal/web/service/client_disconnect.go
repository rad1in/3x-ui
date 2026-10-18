package service

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// DisconnectResult reports a disconnect run: sockets or sessions closed, the
// inbounds whose protocol cannot be kicked, and per-inbound failures.
type DisconnectResult struct {
	Disconnected int               `json:"disconnected"`
	Unsupported  []string          `json:"unsupported"`
	Failed       map[string]string `json:"failed"`
}

func (s *ClientService) recordedClientIPs(email string) []string {
	var row model.InboundClientIps
	if err := database.GetDB().Where("client_email = ?", email).First(&row).Error; err != nil {
		return nil
	}
	var entries []clientIpEntry
	if err := json.Unmarshal([]byte(row.Ips), &entries); err == nil && len(entries) > 0 {
		ips := make([]string, 0, len(entries))
		for _, e := range entries {
			ips = append(ips, e.IP)
		}
		return ips
	}
	var plain []string
	_ = json.Unmarshal([]byte(row.Ips), &plain)
	return plain
}

// DisconnectClient ends the client's live connections on every inbound it is
// attached to, through each inbound's runtime; a node is asked once however
// many of its inbounds carry the client. The client stays enabled.
func (s *ClientService) DisconnectClient(inboundSvc *InboundService, email string) (*DisconnectResult, error) {
	ids, err := s.GetInboundIdsForEmail(database.GetDB(), email)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, common.NewError("client not found or not attached to any inbound:", email)
	}
	var inbounds []*model.Inbound
	if err := database.GetDB().Where("id IN ?", ids).Order("id").Find(&inbounds).Error; err != nil {
		return nil, err
	}

	res := &DisconnectResult{Unsupported: []string{}, Failed: map[string]string{}}
	ips := s.recordedClientIPs(email)
	askedNodes := map[int]bool{}
	for _, ib := range inbounds {
		if ib.NodeID != nil {
			if askedNodes[*ib.NodeID] {
				continue
			}
			askedNodes[*ib.NodeID] = true
		}
		rt, push, _, err := inboundSvc.nodePushPlan(ib)
		if err != nil || !push || rt == nil {
			res.Failed[ib.Tag] = "node unreachable"
			continue
		}
		ctx, cancel := nodePushContext()
		n, err := rt.DisconnectClient(ctx, ib, email, ips)
		cancel()
		switch {
		case errors.Is(err, runtime.ErrDisconnectUnsupported):
			res.Unsupported = append(res.Unsupported, ib.Tag)
		case err != nil:
			res.Failed[ib.Tag] = err.Error()
		default:
			res.Disconnected += n
		}
	}
	sort.Strings(res.Unsupported)
	return res, nil
}
