package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
	"github.com/hnsx-io/hnsx/server/internal/ws/handler"
)

// wsServiceAdapter implements handler.Services against the wired
// application services. R3.5h keeps a single tenant; the WS handler
// reuses the same IssueSvc / ApprovalSvc as the HTTP API so there's
// exactly one source of truth.
type wsServiceAdapter struct {
	issues   *issueServiceHandle
	sink     observation.Sink
	approval *approvalServiceHandle
}

type issueServiceHandle struct {
	svc interface {
		ListByWorkspace(ctx context.Context, workspaceID string, filter issue.ListFilter) ([]*issue.Issue, error)
		UpdateStatus(ctx context.Context, id string, status issue.Status) error
	}
}

type approvalServiceHandle struct {
	svc interface {
		Grant(ctx context.Context, id, userID string) (*approval.Approval, error)
		Deny(ctx context.Context, id, userID string) (*approval.Approval, error)
	}
}

func (a *wsServiceAdapter) ListByWorkspace(ctx context.Context, workspaceID string, filter issue.ListFilter) ([]*issue.Issue, error) {
	return a.issues.svc.ListByWorkspace(ctx, workspaceID, filter)
}

func (a *wsServiceAdapter) UpdateStatus(ctx context.Context, id string, status issue.Status) error {
	return a.issues.svc.UpdateStatus(ctx, id, status)
}

func (a *wsServiceAdapter) WriteObservations(ctx context.Context, obs []*observation.Observation) error {
	if a.sink == nil {
		return errors.New("ws: observation sink not wired")
	}
	for _, o := range obs {
		if err := a.sink.Write(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

func (a *wsServiceAdapter) ApprovalRequest(ctx context.Context, a2 *approval.Approval) error {
	// Not yet wired through the service; reserved for R3.5h+ when
	// daemon-side gate uses the WS to push approval asks to the server.
	return errors.New("ws: approval request not yet routed via WS")
}

func (a *wsServiceAdapter) ApprovalGrant(ctx context.Context, id, userID string) (*approval.Approval, error) {
	return a.approval.svc.Grant(ctx, id, userID)
}

func (a *wsServiceAdapter) ApprovalDeny(ctx context.Context, id, userID string) (*approval.Approval, error) {
	return a.approval.svc.Deny(ctx, id, userID)
}

func (a *wsServiceAdapter) Heartbeat(ctx context.Context, workspaceID string) error {
	// R3.5h: a real heartbeat would write to the daemons table; for
	// now we just log so the WS round-trip is observable.
	slog.Default().Info("ws: daemon heartbeat", "workspace", workspaceID, "ts", time.Now().UTC())
	return nil
}

// _ guards compile-time conformance of the adapter to handler.Services.
var _ handler.Services = (*wsServiceAdapter)(nil)