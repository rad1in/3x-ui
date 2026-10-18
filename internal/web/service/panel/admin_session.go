package panel

import (
	"errors"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var ErrSessionForbidden = errors.New("you can only manage your own sessions")

// SessionView is one signed-in browser of an admin account.
type SessionView struct {
	Id        string `json:"id" example:"q3Zk0m1u0Vw5gq8mYz1lJm0o3Wq2B8Xc"`
	UserId    int    `json:"userId" example:"1"`
	Username  string `json:"username" example:"admin"`
	Ip        string `json:"ip" example:"203.0.113.5"`
	UserAgent string `json:"userAgent" example:"Mozilla/5.0 (X11; Linux x86_64)"`
	CreatedAt int64  `json:"createdAt" example:"1760684400000"`
	LastSeen  int64  `json:"lastSeen" example:"1760688000000"`
	Current   bool   `json:"current" example:"true"`
}

// SessionService lists and ends admin sessions. Owners act on any account,
// everyone else only on their own.
type SessionService struct{}

func mayManage(actor *model.User, userId int) bool {
	return actor.Role == model.AdminRoleOwner || actor.Id == userId
}

// List returns the sessions of userId, or of every account when userId is 0
// and the actor is an owner. maxIdle drops sessions idle longer than the cookie lives.
func (s *SessionService) List(actor *model.User, userId int, currentId string, maxIdle time.Duration) ([]SessionView, error) {
	if actor.Role != model.AdminRoleOwner {
		userId = actor.Id
	}
	q := database.GetDB().Table("admin_sessions AS s").
		Select("s.id, s.user_id, COALESCE(u.username, '') AS username, s.ip, s.user_agent, s.created_at, s.last_seen").
		Joins("LEFT JOIN users u ON u.id = s.user_id").
		Where("s.last_seen >= ?", time.Now().Add(-maxIdle).UnixMilli())
	if userId != 0 {
		q = q.Where("s.user_id = ?", userId)
	}
	out := []SessionView{}
	if err := q.Order("s.last_seen DESC").Scan(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Current = out[i].Id == currentId
	}
	return out, nil
}

func (s *SessionService) Revoke(actor *model.User, id string) (int, error) {
	var row model.AdminSession
	if err := database.GetDB().Where("id = ?", id).Take(&row).Error; err != nil {
		if database.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	if !mayManage(actor, row.UserId) {
		return 0, ErrSessionForbidden
	}
	return row.UserId, database.GetDB().Delete(&model.AdminSession{}, "id = ?", id).Error
}

// RevokeAll signs userId out everywhere (0 means the actor), optionally
// sparing the session making the request.
func (s *SessionService) RevokeAll(actor *model.User, userId int, keepId string) (int64, error) {
	if userId == 0 {
		userId = actor.Id
	}
	if !mayManage(actor, userId) {
		return 0, ErrSessionForbidden
	}
	q := database.GetDB().Where("user_id = ?", userId)
	if keepId != "" {
		q = q.Where("id <> ?", keepId)
	}
	res := q.Delete(&model.AdminSession{})
	return res.RowsAffected, res.Error
}

// PruneStale deletes sessions idle for longer than maxIdle.
func (s *SessionService) PruneStale(maxIdle time.Duration) error {
	return database.GetDB().Where("last_seen < ?", time.Now().Add(-maxIdle).UnixMilli()).Delete(&model.AdminSession{}).Error
}
