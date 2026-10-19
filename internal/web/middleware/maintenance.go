package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DefaultMaintenanceMessage is the notice served when none is configured.
const DefaultMaintenanceMessage = "The service is under maintenance. Please try again later."

// MaintenanceMiddleware answers every request with 503 and the notice while
// maintenance is on. notice is read per request so the switch needs no restart.
func MaintenanceMiddleware(notice func() (on bool, message string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		on, message := notice()
		if !on {
			c.Next()
			return
		}
		if message == "" {
			message = DefaultMaintenanceMessage
		}
		c.Header("Retry-After", "300")
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusServiceUnavailable, "%s", message)
		c.Abort()
	}
}
