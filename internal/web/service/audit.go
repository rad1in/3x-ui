package service

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// AuditActionLogin marks a login attempt row; API rows carry "METHOD /route".
const AuditActionLogin = "login"

// AuditRetention is how long activity-log rows are kept before the daily prune.
const AuditRetention = 90 * 24 * time.Hour

const (
	auditDefaultPageSize = 50
	auditMaxPageSize     = 200
)

// AuditService stores and lists the admin activity log.
type AuditService struct{}

type AuditListParams struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Search   string `form:"search"`
	// Result is "ok", "fail" or empty for both.
	Result string `form:"result"`
}

type AuditLogPage struct {
	Items []model.AuditLog `json:"items"`
	Total int64            `json:"total" example:"120"`
}

func (s *AuditService) Record(entry model.AuditLog) error {
	return database.GetDB().Create(&entry).Error
}

func (s *AuditService) List(p AuditListParams) (*AuditLogPage, error) {
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = auditDefaultPageSize
	}
	pageSize = min(pageSize, auditMaxPageSize)
	page := max(p.Page, 1)

	q := database.GetDB().Model(&model.AuditLog{})
	if needle := strings.TrimSpace(p.Search); needle != "" {
		like := "%" + escapeLikeLiteral(strings.ToLower(needle)) + "%"
		q = q.Where("LOWER(actor) LIKE ? ESCAPE '\\' OR LOWER(ip) LIKE ? ESCAPE '\\' OR LOWER(action) LIKE ? ESCAPE '\\' OR LOWER(target) LIKE ? ESCAPE '\\'",
			like, like, like, like)
	}
	switch p.Result {
	case "ok":
		q = q.Where("success = ?", true)
	case "fail":
		q = q.Where("success = ?", false)
	}

	out := &AuditLogPage{Items: []model.AuditLog{}}
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	err := q.Order("created_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&out.Items).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Prune deletes rows created before the given unix-ms time and reports how many went.
func (s *AuditService) Prune(beforeMs int64) (int64, error) {
	res := database.GetDB().Where("created_at < ?", beforeMs).Delete(&model.AuditLog{})
	return res.RowsAffected, res.Error
}

// HandleEvent is the event-bus subscriber that turns login attempts into rows.
func (s *AuditService) HandleEvent(e eventbus.Event) {
	if e.Type != eventbus.EventLoginAttempt {
		return
	}
	data, ok := e.Data.(*eventbus.LoginEventData)
	if !ok || data == nil {
		return
	}
	err := s.Record(model.AuditLog{
		CreatedAt: e.Timestamp.UnixMilli(),
		Actor:     data.Username,
		Ip:        data.IP,
		Action:    AuditActionLogin,
		Target:    data.Reason,
		Success:   data.Status == "success",
	})
	if err != nil {
		logger.Warning("audit: record login attempt:", err)
	}
}
