package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hnsx-io/hnsx/server/infrastrcutrue/logger"
	"go.uber.org/zap"
)

func ZapLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		size := c.Writer.Size()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("method", c.Request.Method),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", size),
			zap.String("trace_id", c.GetString("trace_id")),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
			logger.Error("Request failed", fields...)
		} else if status >= 500 {
			logger.Error("Server error", fields...)
		} else if status >= 400 {
			logger.Error("Client error", fields...)
		} else {
			logger.Info("Request failed", fields...)
		}
	}
}
