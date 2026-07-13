// Package handler is the shared business kernel for the HTTP API, gRPC control
// plane, and (via type alignment) the CLI client.
//
// Handlers in this package know nothing about gin.Context or connect.Request.
// They accept strongly-typed inputs and return strongly-typed viewmodels from
// pkg/handler/viewmodel. Transport-specific serialization and error mapping
// live in pkg/api, pkg/controlplane, and internal/client.
package handler

import (
	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/commands"
)

// Handler is the root of the shared handler layer.
type Handler struct {
	App             *app.Application
	DomainCommands  *commands.DomainCommands
	SessionCommands *commands.SessionCommands
	Logger          *zap.Logger
}

// New constructs a Handler backed by the supplied Application.
func New(app *app.Application, log *zap.Logger) *Handler {
	h := &Handler{App: app, Logger: log}
	if app != nil {
		h.DomainCommands = commands.NewDomainCommands(app.DomainService)
		h.SessionCommands = commands.NewSessionCommands(app.SessionService, app.DomainService, app.WorkerService, app.State)
	}
	return h
}
