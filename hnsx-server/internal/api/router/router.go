// Package router constructs the gin engine and wires every HTTP handler.
// Layering: router depends on api/handler/* (which depends on service/*)
// and on api/middleware. It does NOT touch domain or infra directly.
package router

import (
	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/api/handler/agent"
	"github.com/hnsx-io/hnsx/server/internal/api/handler/daemon"
	"github.com/hnsx-io/hnsx/server/internal/api/handler/issue"
	"github.com/hnsx-io/hnsx/server/internal/api/handler/squad"
	"github.com/hnsx-io/hnsx/server/internal/api/handler/workspace"
	"github.com/hnsx-io/hnsx/server/internal/api/middleware"
)

// Deps groups the handlers the router wires. app.New constructs this
// once and passes it to New().
type Deps struct {
	Workspace *workspace.Handler
	Issue     *issue.Handler
	Agent     *agent.Handler
	Squad     *squad.Handler
	Daemon    *daemon.Handler
}

// New constructs a fully-configured gin.Engine with all routes mounted
// under /api. Returns the engine ready to serve.
func New(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	deps.Workspace.Register(api)
	deps.Issue.Register(api)
	deps.Agent.Register(api)
	deps.Squad.Register(api)
	deps.Daemon.Register(api)

	return r
}