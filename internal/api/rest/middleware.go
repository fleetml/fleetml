package rest

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"
const requestIDKey = "request_id"

// RequestID generates a unique request ID and adds it to context + response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set(requestIDKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// RequestLogger logs each request with method, path, status, duration, request_id.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(requestIDKey),
		)
	}
}

// ErrorRecovery recovers from panics and returns 500.
func ErrorRecovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"error", r,
					"request_id", c.GetString(requestIDKey),
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatusJSON(500, gin.H{
					"error": gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "An internal error occurred",
					},
				})
			}
		}()
		c.Next()
	}
}
