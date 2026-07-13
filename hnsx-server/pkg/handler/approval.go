package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"

	approvalmodel "github.com/hnsx-io/hnsx/server/internal/approval/model"
	approvalrepo "github.com/hnsx-io/hnsx/server/internal/approval/repository"
	auditmodel "github.com/hnsx-io/hnsx/server/internal/audit/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListApprovalsInput struct {
	TenantID  tenant.ID
	DomainID  string
	SessionID string
	Status    string
}

type GetApprovalInput struct {
	TenantID tenant.ID
	ID       string
}

type CreateApprovalInput struct {
	TenantID    tenant.ID
	ID          string
	SessionID   string
	DomainID    string
	Action      string
	Resource    string
	RiskLevel   string
	Context     map[string]any
	RequestedBy string
}

type ApproveApprovalInput struct {
	TenantID   tenant.ID
	ID         string
	ReviewedBy string
	Comment    string
}

type RejectApprovalInput struct {
	TenantID   tenant.ID
	ID         string
	ReviewedBy string
	Comment    string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListApprovalsOutput struct {
	Approvals viewmodel.ApprovalList
}

type GetApprovalOutput struct {
	Approval *viewmodel.ApprovalDetail
}

type CreateApprovalOutput struct {
	Approval *viewmodel.ApprovalCreated
	Location string
}

type ApproveApprovalOutput struct {
	Approval *viewmodel.ApprovalDetail
}

type RejectApprovalOutput struct {
	Approval *viewmodel.ApprovalDetail
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListApprovals returns approvals matching the supplied filter.
// When Status is empty it defaults to "pending" so the inbox surfaces
// only unresolved gates.
func (h *Handler) ListApprovals(ctx context.Context, in ListApprovalsInput) (*ListApprovalsOutput, error) {
	defer h.hook(ctx, "approval.list",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("domain_id", in.DomainID),
		zap.String("session_id", in.SessionID),
		zap.String("status", in.Status),
	)()

	if h.App == nil || h.App.ApprovalService == nil {
		return nil, approvalmodel.ErrApprovalNotFound
	}

	status := in.Status
	if status == "" {
		status = string(approvalmodel.StatusPending)
	}

	items, err := h.App.ApprovalService.List(approvalrepo.ListFilter{
		DomainID:  in.DomainID,
		SessionID: in.SessionID,
		Status:    status,
	})
	if err != nil {
		return nil, err
	}

	out := make([]viewmodel.ApprovalListItem, 0, len(items))
	for _, it := range items {
		out = append(out, viewmodel.ApprovalListItem{
			ID:          it.ID,
			SessionID:   it.SessionID,
			DomainID:    it.DomainID,
			Action:      it.Action,
			Resource:    it.Resource,
			RiskLevel:   string(it.RiskLevel),
			Status:      string(it.Status),
			RequestedBy: it.RequestedBy,
			CreatedAt:   it.CreatedAt,
			UpdatedAt:   it.UpdatedAt,
		})
	}

	return &ListApprovalsOutput{Approvals: viewmodel.ApprovalList{
		Items: out,
		Total: len(out),
	}}, nil
}

// GetApproval returns a single approval detail.
func (h *Handler) GetApproval(ctx context.Context, in GetApprovalInput) (*GetApprovalOutput, error) {
	defer h.hook(ctx, "approval.get",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("id", in.ID),
	)()

	if h.App == nil || h.App.ApprovalService == nil {
		return nil, approvalmodel.ErrApprovalNotFound
	}

	a, err := h.App.ApprovalService.Get(in.ID)
	if err != nil {
		return nil, err
	}

	return &GetApprovalOutput{Approval: toApprovalDetail(a)}, nil
}

// CreateApproval persists a new human-approval gate.
func (h *Handler) CreateApproval(ctx context.Context, in CreateApprovalInput) (*CreateApprovalOutput, error) {
	defer h.hook(ctx, "approval.create",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
		zap.String("domain_id", in.DomainID),
	)()

	if h.App == nil || h.App.ApprovalService == nil {
		return nil, approvalmodel.ErrApprovalNotFound
	}

	id := in.ID
	if id == "" {
		id = approvalmodel.NewID(in.SessionID)
	}

	risk := approvalmodel.RiskLevel(in.RiskLevel)
	if risk == "" {
		risk = approvalmodel.RiskHigh
	}

	a := &approvalmodel.Approval{
		ID:          id,
		SessionID:   in.SessionID,
		DomainID:    in.DomainID,
		Action:      in.Action,
		Resource:    in.Resource,
		RiskLevel:   risk,
		Context:     in.Context,
		RequestedBy: in.RequestedBy,
	}

	if err := h.App.ApprovalService.Create(a); err != nil {
		return nil, err
	}

	return &CreateApprovalOutput{
		Approval: toApprovalCreated(a),
		Location: "/api/v1/approvals/" + a.ID,
	}, nil
}

// ApproveApproval resolves an approval as approved.
func (h *Handler) ApproveApproval(ctx context.Context, in ApproveApprovalInput) (*ApproveApprovalOutput, error) {
	defer h.hook(ctx, "approval.approve",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("id", in.ID),
		zap.String("reviewed_by", in.ReviewedBy),
	)()

	if h.App == nil || h.App.ApprovalService == nil {
		return nil, approvalmodel.ErrApprovalNotFound
	}

	reviewer := in.ReviewedBy
	if reviewer == "" {
		reviewer = "operator"
	}

	got, err := h.App.ApprovalService.Approve(in.ID, reviewer, in.Comment)
	if err != nil {
		return nil, err
	}

	h.recordApprovalDecision(ctx, got, "approved", reviewer, in.Comment)

	return &ApproveApprovalOutput{Approval: toApprovalDetail(got)}, nil
}

// RejectApproval resolves an approval as rejected.
func (h *Handler) RejectApproval(ctx context.Context, in RejectApprovalInput) (*RejectApprovalOutput, error) {
	defer h.hook(ctx, "approval.reject",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("id", in.ID),
		zap.String("reviewed_by", in.ReviewedBy),
	)()

	if h.App == nil || h.App.ApprovalService == nil {
		return nil, approvalmodel.ErrApprovalNotFound
	}

	reviewer := in.ReviewedBy
	if reviewer == "" {
		reviewer = "operator"
	}

	got, err := h.App.ApprovalService.Reject(in.ID, reviewer, in.Comment)
	if err != nil {
		return nil, err
	}

	h.recordApprovalDecision(ctx, got, "rejected", reviewer, in.Comment)

	return &RejectApprovalOutput{Approval: toApprovalDetail(got)}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toApprovalDetail(a *approvalmodel.Approval) *viewmodel.ApprovalDetail {
	if a == nil {
		return nil
	}
	return &viewmodel.ApprovalDetail{
		ID:          a.ID,
		SessionID:   a.SessionID,
		DomainID:    a.DomainID,
		Action:      a.Action,
		Resource:    a.Resource,
		RiskLevel:   string(a.RiskLevel),
		Context:     a.Context,
		Status:      string(a.Status),
		RequestedBy: a.RequestedBy,
		ReviewedBy:  a.ReviewedBy,
		Comment:     a.Comment,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		ResolvedAt:  a.ResolvedAt,
	}
}

func toApprovalCreated(a *approvalmodel.Approval) *viewmodel.ApprovalCreated {
	if a == nil {
		return nil
	}
	return &viewmodel.ApprovalCreated{
		ID:          a.ID,
		SessionID:   a.SessionID,
		DomainID:    a.DomainID,
		Action:      a.Action,
		Resource:    a.Resource,
		RiskLevel:   string(a.RiskLevel),
		Context:     a.Context,
		Status:      string(a.Status),
		RequestedBy: a.RequestedBy,
		ReviewedBy:  a.ReviewedBy,
		Comment:     a.Comment,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		ResolvedAt:  a.ResolvedAt,
	}
}

// recordApprovalDecision writes an immutable audit row alongside each
// approval decision so the AuditLog can attribute human-gate changes.
func (h *Handler) recordApprovalDecision(ctx context.Context, a *approvalmodel.Approval, decision, reviewer, comment string) {
	if h.App == nil || h.App.AuditService == nil || a == nil {
		return
	}

	entry := auditmodel.Entry{
		SessionID: a.SessionID,
		DomainID:  a.DomainID,
		Action:    "approval_decision",
		Actor:     reviewer,
		ActorType: auditmodel.ActorTypeUser,
		Resource:  "approval:" + a.ID,
		Decision:  decision,
		Reason:    comment,
		Details: map[string]any{
			"approval_id": a.ID,
			"action":      a.Action,
			"resource":    a.Resource,
			"risk_level":  a.RiskLevel,
		},
	}
	_ = h.App.AuditService.Record(ctx, &entry)
}

// ErrApprovalNotFound is re-exported from the approval model so HTTP/gRPC
// layers can compare errors directly.
var ErrApprovalNotFound = approvalmodel.ErrApprovalNotFound

// ErrApprovalAlreadyResolved is re-exported from the approval model.
var ErrApprovalAlreadyResolved = approvalmodel.ErrAlreadyResolved

// IsApprovalNotFound reports whether err is an approval-not-found error.
func IsApprovalNotFound(err error) bool {
	return errors.Is(err, approvalmodel.ErrApprovalNotFound)
}

// IsApprovalAlreadyResolved reports whether err is an already-resolved error.
func IsApprovalAlreadyResolved(err error) bool {
	return errors.Is(err, approvalmodel.ErrAlreadyResolved)
}
