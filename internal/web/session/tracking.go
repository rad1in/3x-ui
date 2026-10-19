package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	// LastSeen is refreshed at most this often, so a busy page does not write per request.
	lastSeenGranularity = time.Minute
	userAgentMaxLen     = 255
)

func newSessionId() (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func registerSession(c *gin.Context, userId int) (string, error) {
	id, err := newSessionId()
	if err != nil {
		return "", err
	}
	now := time.Now().UnixMilli()
	row := model.AdminSession{
		Id: id, UserId: userId, Ip: ClientIP(c),
		UserAgent: clip(c.Request.UserAgent(), userAgentMaxLen),
		CreatedAt: now, LastSeen: now,
	}
	return id, database.GetDB().Create(&row).Error
}

// touchSession reports whether the cookie's session is still alive and keeps
// its last-seen time and address current. A cookie that predates session
// tracking is registered on first sight, so it can be listed and revoked.
func touchSession(c *gin.Context, s sessions.Session, userId int) bool {
	db := database.GetDB()
	id, _ := s.Get(loginSessionKey).(string)
	if id == "" {
		newId, err := registerSession(c, userId)
		if err != nil {
			logger.Warning("session: failed to register legacy session:", err)
			return true
		}
		s.Set(loginSessionKey, newId)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to store legacy session id:", err)
		}
		return true
	}
	var row model.AdminSession
	err := db.Where("id = ? AND user_id = ?", id, userId).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		logger.Warning("session: failed to read session:", err)
		return true
	}
	now := time.Now().UnixMilli()
	ip := ClientIP(c)
	if now-row.LastSeen < lastSeenGranularity.Milliseconds() && row.Ip == ip {
		return true
	}
	err = db.Model(&model.AdminSession{}).Where("id = ?", id).
		Updates(map[string]any{"last_seen": now, "ip": ip}).Error
	if err != nil {
		logger.Warning("session: failed to refresh session:", err)
	}
	return true
}

// CurrentSessionId is the id of the cookie session making this request.
func CurrentSessionId(c *gin.Context) string {
	id, _ := sessions.Default(c).Get(loginSessionKey).(string)
	return id
}

func dropCurrentSession(c *gin.Context) {
	if id := CurrentSessionId(c); id != "" {
		if err := database.GetDB().Delete(&model.AdminSession{}, "id = ?", id).Error; err != nil {
			logger.Warning("session: failed to delete session on logout:", err)
		}
	}
}
