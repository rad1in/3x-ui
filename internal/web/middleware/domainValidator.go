// Package middleware provides HTTP middleware functions for the 3x-ui web panel,
// including domain validation utilities.
package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DomainValidatorMiddleware lets through only requests for the configured domain.
// Others get reject when it is set (the decoy site), else 403 Forbidden.
func DomainValidatorMiddleware(domain string, reject gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.Request.Host
		if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
			host, _, _ = net.SplitHostPort(c.Request.Host)
		}

		if host != domain {
			if reject != nil {
				reject(c)
				c.Abort()
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
