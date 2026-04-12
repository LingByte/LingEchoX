package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LoggerMiddleware 请求日志中间件
func LoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		method := c.Request.Method

		// 处理请求
		c.Next()

		// Filter rules:
		// 1. Filter monitoring-related paths (/metrics, /monitor, etc.)
		shouldLog := true

		// Filter monitoring-related paths
		if strings.Contains(path, "/metrics") ||
			strings.Contains(path, "/monitor") ||
			strings.Contains(path, "/static") ||
			strings.Contains(path, "/favicon.ico") {
			shouldLog = false
		}

		// Log request
		if shouldLog {
			end := time.Now()
			latency := end.Sub(start)
			logger.Info("Request",
				zap.Int("status", c.Writer.Status()),
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.String("ip", c.ClientIP()),
				zap.String("user-agent", c.Request.UserAgent()),
				zap.Duration("latency", latency),
			)
		}
	}
}
