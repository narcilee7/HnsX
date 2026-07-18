package ws

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	wsserver "github.com/hnsx-io/hnsx/server/internal/ws"
	"github.com/hnsx-io/hnsx/server/internal/ws/handler"
)

// Handler is the gin route that upgrades a /ws/daemon HTTP request
// to a WebSocket and hands the conn to the ws package.
func Handler(daemonHandler *handler.Handler, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsConn, err := wsserver.ServerUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Warn("ws: upgrade failed", "err", err)
			return
		}
		_ = wsserver.NewServerConn(wsConn, daemonHandler, logger)
		<-c.Request.Context().Done()
	}
}
