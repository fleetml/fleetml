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
// Server errors are logged at error level with the underlying cause.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(requestIDKey),
		}
		if c.Writer.Status() >= 500 {
			if len(c.Errors) > 0 {
				attrs = append(attrs, "error", c.Errors.String())
			}
			logger.Error("request failed", attrs...)
			return
		}
		logger.Info("request", attrs...)
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
