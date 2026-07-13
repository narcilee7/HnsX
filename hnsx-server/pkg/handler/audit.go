package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"

	auditmodel "github.com/hnsx-io/hnsx/server/internal/audit/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// ListAuditInput carries the parameters for paginated audit entry listing.
type ListAuditInput struct {
	TenantID tenant.ID
	Limit    int
	Offset   int
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

// ListAuditOutput is the result of listing audit entries.
type ListAuditOutput struct {
	Entries viewmodel.AuditList
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListAudit returns a paginated list of immutable audit entries.
func (h *Handler) ListAudit(ctx context.Context, in ListAuditInput) (*ListAuditOutput, error) {
	defer h.hook(ctx, "audit.list", zap.String("tenant_id", string(in.TenantID)))()

	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}

	if h.App == nil || h.App.AuditService == nil {
		return &ListAuditOutput{Entries: viewmodel.AuditList{
			Items:  []viewmodel.AuditListItem{},
			Total:  0,
			Limit:  limit,
			Offset: in.Offset,
		}}, nil
	}

	entries, total, err := h.App.AuditService.List(limit, in.Offset)
	if err != nil {
		return nil, h.mapAuditError(err)
	}

	out := make([]viewmodel.AuditListItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, viewmodel.AuditListItem{
			ID:           e.ID,
			SessionID:    e.SessionID,
			DomainID:     e.DomainID,
			Action:       e.Action,
			Actor:        e.Actor,
			ActorType:    e.ActorType,
			Resource:     e.Resource,
			ResourceType: e.ResourceType,
			Decision:     e.Decision,
			Reason:       e.Reason,
			Details:      e.Details,
			Timestamp:    e.Timestamp,
		})
	}

	return &ListAuditOutput{Entries: viewmodel.AuditList{
		Items:  out,
		Total:  total,
		Limit:  limit,
		Offset: in.Offset,
	}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ErrAuditEntryNotFound is re-exported from the audit model so transport
// layers can compare errors directly.
var ErrAuditEntryNotFound = auditmodel.ErrAuditEntryNotFound

// IsAuditEntryNotFound reports whether err is an audit-entry-not-found error.
func IsAuditEntryNotFound(err error) bool {
	return errors.Is(err, auditmodel.ErrAuditEntryNotFound)
}

// mapAuditError maps internal audit errors to the handler-level error contract.
func (h *Handler) mapAuditError(err error) error {
	if err == nil {
		return nil
	}
	return err
}
