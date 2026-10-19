package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// The sensitive actions a second admin can be required to approve. Keep this
// list small: each needs an executor below that re-runs it through its normal service.
const (
	ApprovalInboundDelete     = "inbound.delete"
	ApprovalClientsBulkDelete = "clients.bulkDelete"
	ApprovalNodeDelete        = "node.delete"
)

// ApprovalActions is the whole gateable set, in display order.
var ApprovalActions = []string{ApprovalInboundDelete, ApprovalClientsBulkDelete, ApprovalNodeDelete}

const (
	ApprovalPending   = "pending"
	ApprovalApproved  = "approved"
	ApprovalRejected  = "rejected"
	ApprovalCancelled = "cancelled"
	ApprovalExpired   = "expired"
	ApprovalFailed    = "failed"

	approvalTTL = 24 * time.Hour
)

var (
	ErrApprovalForbidden = errors.New("only an owner or admin can decide approvals")
	ErrApprovalSelf      = errors.New("an action cannot be approved or rejected by the admin who requested it")
	ErrApprovalNotFound  = errors.New("approval request not found")
	ErrApprovalClosed    = errors.New("approval request is no longer pending")
	ErrApprovalUnknown   = errors.New("unknown approval action")
)

// Payload shapes the executors read.

type InboundDeleteApproval struct {
	Ids []int `json:"ids"`
}

type ClientsBulkDeleteApproval struct {
	Emails      []string `json:"emails"`
	KeepTraffic bool     `json:"keepTraffic"`
}

type NodeDeleteApproval struct {
	Id int `json:"id"`
}

// ApprovalService parks gated actions and runs them once a second admin agrees.
type ApprovalService struct {
	audit AuditService
	now   func() time.Time
	// exec maps an action to the call that performs it; tests swap entries.
	exec map[string]func(payload json.RawMessage) (string, error)
}

func NewApprovalService() *ApprovalService {
	s := &ApprovalService{now: time.Now}
	s.exec = map[string]func(json.RawMessage) (string, error){
		ApprovalInboundDelete:     s.execInboundDelete,
		ApprovalClientsBulkDelete: s.execClientsBulkDelete,
		ApprovalNodeDelete:        s.execNodeDelete,
	}
	return s
}

func canDecide(u *model.User) bool {
	return u != nil && (u.Role == model.AdminRoleOwner || u.Role == model.AdminRoleAdmin)
}

// Required reports whether this actor's request for action must wait for a
// second admin. It is false when no other owner or admin exists, so enabling
// the gate on a single-admin panel never locks that admin out.
func (s *ApprovalService) Required(actor *model.User, action string) (bool, error) {
	cfg, err := (&SecurityConfigService{}).Get()
	if err != nil {
		return false, err
	}
	if !slices.Contains(cfg.ApprovalActions, action) {
		return false, nil
	}
	var others int64
	err = database.GetDB().Model(&model.User{}).
		Where("role IN ? AND id <> ?", []string{model.AdminRoleOwner, model.AdminRoleAdmin}, actor.Id).
		Count(&others).Error
	return others > 0, err
}

// Request parks the action and records it in the activity log.
func (s *ApprovalService) Request(actor *model.User, ip, action, target string, payload any) (*model.PendingAction, error) {
	if _, ok := s.exec[action]; !ok {
		return nil, ErrApprovalUnknown
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	now := s.now()
	row := &model.PendingAction{
		Action: action, Target: target, Payload: string(raw),
		RequestedBy: actor.Id, RequestedByName: actor.Username,
		RequestedAt: now.UnixMilli(), ExpiresAt: now.Add(approvalTTL).UnixMilli(),
		Status: ApprovalPending,
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		return nil, err
	}
	s.record(actor.Username, ip, "approval.requested", row, true)
	return row, nil
}

func (s *ApprovalService) record(actor, ip, event string, row *model.PendingAction, ok bool) {
	err := s.audit.Record(model.AuditLog{
		CreatedAt: s.now().UnixMilli(), Actor: actor, Ip: ip, Action: event,
		Target: fmt.Sprintf("#%d %s %s", row.Id, row.Action, row.Target), Success: ok,
	})
	if err != nil {
		logger.Warning("approval: audit record:", err)
	}
}

// expireStale flips overdue pending rows so lists and decisions agree.
func (s *ApprovalService) expireStale() error {
	return database.GetDB().Model(&model.PendingAction{}).
		Where("status = ? AND expires_at <= ?", ApprovalPending, s.now().UnixMilli()).
		Update("status", ApprovalExpired).Error
}

// List returns open requests first, then the most recent decided ones.
func (s *ApprovalService) List(actor *model.User) ([]model.PendingAction, error) {
	if !canDecide(actor) && actor.Role != model.AdminRoleViewer {
		return nil, ErrApprovalForbidden
	}
	if err := s.expireStale(); err != nil {
		return nil, err
	}
	out := []model.PendingAction{}
	err := database.GetDB().
		Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, id DESC").
		Limit(200).Find(&out).Error
	return out, err
}

func (s *ApprovalService) load(id int) (*model.PendingAction, error) {
	var row model.PendingAction
	if err := database.GetDB().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApprovalNotFound
		}
		return nil, err
	}
	return &row, nil
}

// claim moves a pending row to status in one conditional update, so two
// admins deciding at once cannot both win.
func (s *ApprovalService) claim(id int, actor *model.User, status string) error {
	res := database.GetDB().Model(&model.PendingAction{}).
		Where("id = ? AND status = ? AND expires_at > ?", id, ApprovalPending, s.now().UnixMilli()).
		Updates(map[string]any{"status": status, "decided_by": actor.Id, "decided_by_name": actor.Username, "decided_at": s.now().UnixMilli()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrApprovalClosed
	}
	return nil
}

// Approve runs the action as the approver's decision. The requester can
// never approve their own request.
func (s *ApprovalService) Approve(actor *model.User, ip string, id int) (*model.PendingAction, error) {
	row, err := s.decidable(actor, id)
	if err != nil {
		return nil, err
	}
	if err := s.claim(id, actor, ApprovalApproved); err != nil {
		return nil, err
	}
	result, execErr := s.exec[row.Action](json.RawMessage(row.Payload))
	status := ApprovalApproved
	if execErr != nil {
		status, result = ApprovalFailed, execErr.Error()
	}
	if err := database.GetDB().Model(&model.PendingAction{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "result": result}).Error; err != nil {
		logger.Warning("approval: store result:", err)
	}
	row, _ = s.load(id)
	s.record(actor.Username, ip, "approval.approved", row, execErr == nil)
	return row, execErr
}

// Reject drops the request without running it.
func (s *ApprovalService) Reject(actor *model.User, ip string, id int) (*model.PendingAction, error) {
	if _, err := s.decidable(actor, id); err != nil {
		return nil, err
	}
	if err := s.claim(id, actor, ApprovalRejected); err != nil {
		return nil, err
	}
	row, _ := s.load(id)
	s.record(actor.Username, ip, "approval.rejected", row, true)
	return row, nil
}

// Cancel lets the requester withdraw their own pending request.
func (s *ApprovalService) Cancel(actor *model.User, ip string, id int) (*model.PendingAction, error) {
	row, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if row.RequestedBy != actor.Id {
		return nil, ErrApprovalForbidden
	}
	if err := s.claim(id, actor, ApprovalCancelled); err != nil {
		return nil, err
	}
	row, _ = s.load(id)
	s.record(actor.Username, ip, "approval.cancelled", row, true)
	return row, nil
}

func (s *ApprovalService) decidable(actor *model.User, id int) (*model.PendingAction, error) {
	if !canDecide(actor) {
		return nil, ErrApprovalForbidden
	}
	row, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if row.RequestedBy == actor.Id {
		return nil, ErrApprovalSelf
	}
	return row, nil
}

func (s *ApprovalService) execInboundDelete(raw json.RawMessage) (string, error) {
	var p InboundDeleteApproval
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	inbounds := &InboundService{}
	var needRestart bool
	var err error
	skipped := ""
	if len(p.Ids) == 1 {
		needRestart, err = inbounds.DelInbound(p.Ids[0])
	} else {
		var res BulkDelInboundResult
		res, needRestart, err = inbounds.DelInbounds(p.Ids)
		for _, r := range res.Skipped {
			skipped += fmt.Sprintf(" #%d: %s;", r.Id, r.Reason)
		}
		if err == nil {
			skipped = fmt.Sprintf("deleted %d.%s", res.Deleted, skipped)
		}
	}
	if needRestart {
		(&XrayService{}).SetToNeedRestart()
	}
	return strings.TrimSpace(skipped), err
}

func (s *ApprovalService) execClientsBulkDelete(raw json.RawMessage) (string, error) {
	var p ClientsBulkDeleteApproval
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	inbounds := &InboundService{}
	res, needRestart, err := (&ClientService{}).BulkDelete(inbounds, p.Emails, p.KeepTraffic)
	if needRestart {
		(&XrayService{}).SetToNeedRestart()
	}
	return fmt.Sprintf("deleted %d, skipped %d", res.Deleted, len(res.Skipped)), err
}

func (s *ApprovalService) execNodeDelete(raw json.RawMessage) (string, error) {
	var p NodeDeleteApproval
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	return "", (&NodeService{}).Delete(p.Id)
}
