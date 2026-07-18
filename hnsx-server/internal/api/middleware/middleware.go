// Package middleware hosts HTTP middleware shared across handlers.
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery catches panics in downstream handlers and turns them into
// 500 responses. Logs the panic + stack via slog so we don't lose
// diagnostic info when gin recovers.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic in handler",
			slog.Any("recovered", recovered),
			slog.String("path", c.Request.URL.Path),
			slog.String("method", c.Request.Method),
		)
		c.AbortWithStatus(500)
	})
}

// Logger emits one structured log line per request. Format matches the
// rest of the application (slog text handler on stderr) so production log
// aggregators can parse uniformly.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("dur", time.Since(start)),
		)
	}
}