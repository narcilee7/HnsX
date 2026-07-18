// Package handler provides the ws.Handler implementation that bridges
// the daemon ↔ server WS protocol to the application services.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
	"github.com/hnsx-io/hnsx/server/internal/ws"
)

// Services is the surface this package needs from app. Defined as a
// narrow interface so we can wire it in tests with stubs.
type Services interface {
	ListAssignedToAgent(ctx context.Context, agentID string, statuses []issue.Status) ([]*issue.Issue, error)
	UpdateStatus(ctx context.Context, id string, status issue.Status) error
	WriteObservations(ctx context.Context, obs []*observation.Observation) error
	ApprovalRequest(ctx context.Context, a *approval.Approval) error
	ApprovalGrant(ctx context.Context, id, userID string) (*approval.Approval, error)
	ApprovalDeny(ctx context.Context, id, userID string) (*approval.Approval, error)
	Heartbeat(ctx context.Context, workspaceID string) error
}

// Handler implements ws.Handler against Services.
type Handler struct {
	svc   Services
	log   *slog.Logger
}

// NewHandler wires a Services implementation to a ws.Handler.
func NewHandler(svc Services, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log}
}

// HandleClaim returns the issues currently assigned to the daemon's
// workspace, filtered to todo / in_progress.
func (h *Handler) HandleClaim(ctx context.Context, req ws.ClaimRequest) (ws.IssuesResponse, error) {
	if req.WorkspaceID == "" {
		return ws.IssuesResponse{}, fmt.Errorf("workspace_id required")
	}
	// R3.5h: claim is workspace-scoped; the daemon iterates agents
	// itself. Here we just list the workspace's issues.
	// R3.5h+ refines to per-agent claim via the daemon's own
	// AgentSvc.ListByWorkspace lookup.
	items, err := h.svc.ListAssignedToAgent(ctx, req.WorkspaceID, []issue.Status{
		issue.StatusTodo, issue.StatusInProgress,
	})
	if err != nil {
		return ws.IssuesResponse{}, err
	}
	resp := ws.IssuesResponse{Items: make([]ws.ClaimedIssue, 0, len(items))}
	for _, i := range items {
		backend := ""
		if i.AssigneeID != nil {
			backend = *i.AssigneeID
		}
		resp.Items = append(resp.Items, ws.ClaimedIssue{
			ID:          i.ID,
			WorkspaceID: i.WorkspaceID,
			Title:       i.Title,
			Description: i.Description,
			AssigneeID:  strDeref(i.AssigneeID),
			AgentID:     strDeref(i.AssigneeID),
			Backend:     backend,
		})
	}
	return resp, nil
}

// HandleObservations persists a batch of observations.
func (h *Handler) HandleObservations(ctx context.Context, batch []ws.ObservationEvent) error {
	out := make([]*observation.Observation, 0, len(batch))
	for _, e := range batch {
		occurred, err := time.Parse(time.RFC3339, e.OccurredAt)
		if err != nil {
			occurred = time.Now().UTC()
		}
		obs := &observation.Observation{
			ID:              e.WorkspaceID, // populated by server-side; placeholder
			WorkspaceID:     e.WorkspaceID,
			IssueID:         e.IssueID,
			AgentID:         e.AgentID,
			Kind:            observation.Kind(e.Kind),
			Sequence:        e.Sequence,
			Payload:         e.Payload,
			OccurredAt:      occurred,
			PromptHash:      e.PromptHash,
			AgentTemplateID: e.AgentTemplateID,
			ToolSignatures:  e.ToolSignatures,
		}
		out = append(out, obs)
	}
	return h.svc.WriteObservations(ctx, out)
}

// HandleIssueStatus updates an issue's status.
func (h *Handler) HandleIssueStatus(ctx context.Context, evt ws.IssueStatusEvent) error {
	return h.svc.UpdateStatus(ctx, evt.IssueID, issue.Status(evt.Status))
}

// HandleApprovalReply routes the daemon's outcome to the approval
// service (grant or deny).
func (h *Handler) HandleApprovalReply(ctx context.Context, rep ws.ApprovalReplyEvent) error {
	switch rep.Decision {
	case "granted":
		_, err := h.svc.ApprovalGrant(ctx, rep.ApprovalID, "daemon")
		return err
	case "denied":
		_, err := h.svc.ApprovalDeny(ctx, rep.ApprovalID, "daemon")
		return err
	default:
		return fmt.Errorf("approval_reply: unknown decision %q", rep.Decision)
	}
}

// HandleHeartbeat records the daemon's liveness signal.
func (h *Handler) HandleHeartbeat(ctx context.Context, workspaceID string) error {
	return h.svc.Heartbeat(ctx, workspaceID)
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// _ ensures the imports stay live.
var _ = json.Marshal