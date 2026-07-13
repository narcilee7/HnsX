package handler

import (
	"context"
	"errors"
	"io"
	"sort"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/obs"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListDomainsInput struct {
	TenantID tenant.ID
	Limit    int
	Offset   int
}

type GetDomainInput struct {
	TenantID tenant.ID
	ID       string
}

type RegisterDomainInput struct {
	TenantID    tenant.ID
	Body        io.Reader
	ContentType string
}

type UpdateDomainInput struct {
	TenantID    tenant.ID
	ID          string
	Body        io.Reader
	ContentType string
}

type DeleteDomainInput struct {
	TenantID tenant.ID
	ID       string
}

type ListDomainVersionsInput struct {
	TenantID tenant.ID
	ID       string
}

type GetDomainVersionInput struct {
	TenantID tenant.ID
	ID       string
	Version  string
}

type GetDomainSchemaInput struct {
	TenantID tenant.ID
	ID       string
}

type ValidateDomainInput struct {
	Body        io.Reader
	ContentType string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListDomainsOutput struct {
	Domains viewmodel.DomainList
}

type GetDomainOutput struct {
	Domain *viewmodel.DomainDetail
}

type RegisterDomainOutput struct {
	Domain    *viewmodel.DomainRegistered
	CreatedAt interface{}
}

type UpdateDomainOutput struct {
	Domain    *viewmodel.DomainRegistered
	UpdatedAt interface{}
}

type ListDomainVersionsOutput struct {
	Versions viewmodel.DomainVersionList
}

type GetDomainVersionOutput struct {
	Domain *viewmodel.DomainDetail
}

type GetDomainSchemaOutput struct {
	Schema *viewmodel.DomainSchemaView
}

type ValidateDomainOutput struct {
	Summary *viewmodel.DomainValidationSummary
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListDomains returns all registered domains for a tenant.
func (h *Handler) ListDomains(ctx context.Context, in ListDomainsInput) (*ListDomainsOutput, error) {
	defer h.hook(ctx, "domain.list", zap.String("tenant_id", string(in.TenantID)))()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	items, err := h.App.DomainService.List(in.TenantID)
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	out := make([]viewmodel.DomainListItem, 0, len(items))
	for _, d := range items {
		var spec *domain.DomainSpec
		if d.Spec != nil {
			spec = d.Spec
		}
		out = append(out, viewmodel.DomainListItem{
			ID:          d.ID,
			Version:     d.Version,
			Description: d.Description,
			Status:      "active",
			Spec:        spec,
			CreatedAt:   d.CreatedAt,
			UpdatedAt:   d.UpdatedAt,
		})
	}

	limit := in.Limit
	if limit <= 0 {
		limit = len(out)
	}
	return &ListDomainsOutput{Domains: viewmodel.DomainList{
		Items:  out,
		Total:  len(out),
		Limit:  limit,
		Offset: in.Offset,
	}}, nil
}

// GetDomain returns a single domain detail.
func (h *Handler) GetDomain(ctx context.Context, in GetDomainInput) (*GetDomainOutput, error) {
	defer h.hook(ctx, "domain.get", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID))()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	d, err := h.App.DomainService.Get(in.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	return &GetDomainOutput{Domain: h.toDomainDetail(d)}, nil
}

// RegisterDomain validates and persists a new domain spec.
func (h *Handler) RegisterDomain(ctx context.Context, in RegisterDomainInput) (*RegisterDomainOutput, error) {
	defer h.hook(ctx, "domain.register", zap.String("tenant_id", string(in.TenantID)))()

	ds, err := domain.DecodeDomainSpec(in.Body, in.ContentType)
	if err != nil {
		return nil, err
	}
	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	d, err := h.App.DomainService.Register(in.TenantID, ds)
	if err != nil {
		return nil, err
	}
	return &RegisterDomainOutput{
		Domain: &viewmodel.DomainRegistered{
			ID:        d.ID,
			Version:   d.Version,
			CreatedAt: d.CreatedAt,
		},
		CreatedAt: d.CreatedAt,
	}, nil
}

// UpdateDomain replaces an existing domain spec.
func (h *Handler) UpdateDomain(ctx context.Context, in UpdateDomainInput) (*UpdateDomainOutput, error) {
	defer h.hook(ctx, "domain.update", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID))()

	ds, err := domain.DecodeDomainSpec(in.Body, in.ContentType)
	if err != nil {
		return nil, err
	}
	if ds.ID != in.ID {
		return nil, commands.ErrIDMismatch
	}
	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	d, err := h.App.DomainService.Update(in.TenantID, in.ID, ds)
	if err != nil {
		return nil, err
	}
	return &UpdateDomainOutput{
		Domain: &viewmodel.DomainRegistered{
			ID:        d.ID,
			Version:   d.Version,
			UpdatedAt: d.UpdatedAt,
		},
		UpdatedAt: d.UpdatedAt,
	}, nil
}

// DeleteDomain removes a domain.
func (h *Handler) DeleteDomain(ctx context.Context, in DeleteDomainInput) error {
	defer h.hook(ctx, "domain.delete", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID))()

	if h.App == nil || h.App.DomainService == nil {
		return domainmodel.ErrDomainNotFound
	}
	return h.App.DomainService.Delete(in.TenantID, in.ID)
}

// ListDomainVersions returns all stored versions for a domain.
func (h *Handler) ListDomainVersions(ctx context.Context, in ListDomainVersionsInput) (*ListDomainVersionsOutput, error) {
	defer h.hook(ctx, "domain.versions.list", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID))()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	d, err := h.App.DomainService.Get(in.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	records, err := h.App.DomainService.ListVersions(in.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	out := make([]viewmodel.DomainVersionItem, 0, len(records))
	for _, r := range records {
		out = append(out, viewmodel.DomainVersionItem{
			Version:   r.Version,
			CreatedAt: r.CreatedAt,
			IsCurrent: r.Version == d.Version,
		})
	}
	return &ListDomainVersionsOutput{Versions: viewmodel.DomainVersionList{
		Items:  out,
		Total:  len(out),
		Limit:  len(out),
		Offset: 0,
	}}, nil
}

// GetDomainVersion returns a specific domain version.
func (h *Handler) GetDomainVersion(ctx context.Context, in GetDomainVersionInput) (*GetDomainVersionOutput, error) {
	defer h.hook(ctx, "domain.version.get", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID), zap.String("version", in.Version))()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	if _, err := h.App.DomainService.Get(in.TenantID, in.ID); err != nil {
		return nil, err
	}
	records, err := h.App.DomainService.ListVersions(in.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Version == in.Version {
			return &GetDomainVersionOutput{Domain: &viewmodel.DomainDetail{
				ID:          in.ID,
				Version:     r.Version,
				Description: r.Spec.Description,
				Harness:     r.Spec.Harness,
				Spec:        r.Spec,
				Status:      "active",
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.CreatedAt,
			}}, nil
		}
	}
	return nil, domainmodel.ErrDomainNotFound
}

// GetDomainSchema returns the workspace schema view of a domain.
func (h *Handler) GetDomainSchema(ctx context.Context, in GetDomainSchemaInput) (*GetDomainSchemaOutput, error) {
	defer h.hook(ctx, "domain.schema.get", zap.String("tenant_id", string(in.TenantID)), zap.String("id", in.ID))()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	d, err := h.App.DomainService.Get(in.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	if d.Spec == nil {
		return nil, domainmodel.ErrInvalidSpec
	}
	return &GetDomainSchemaOutput{Schema: &viewmodel.DomainSchemaView{
		ID:            d.ID,
		Version:       d.Version,
		Mode:          string(d.Spec.Harness.Session.Mode),
		Agent:         d.Spec.Harness.Session.Agent,
		TriggerSchema: d.Spec.Harness.Session.TriggerSchema,
		OutputSchema:  d.Spec.Harness.Session.OutputSchema,
	}}, nil
}

// ValidateDomain validates a domain spec without persisting it.
func (h *Handler) ValidateDomain(ctx context.Context, in ValidateDomainInput) (*ValidateDomainOutput, error) {
	defer h.hook(ctx, "domain.validate")()

	ds, err := domain.DecodeDomainSpec(in.Body, in.ContentType)
	if err != nil {
		return nil, err
	}
	if err := domain.Validate(ds); err != nil {
		return nil, err
	}
	agentCount := 0
	if ds.Harness.Agents != nil {
		agentCount = len(ds.Harness.Agents)
	}
	stepCount := 0
	if ds.Harness.Session.Workflow != nil && ds.Harness.Session.Workflow.Steps != nil {
		stepCount = len(ds.Harness.Session.Workflow.Steps)
	}
	return &ValidateDomainOutput{Summary: &viewmodel.DomainValidationSummary{
		Valid:      true,
		ID:         ds.ID,
		Version:    ds.Version,
		Mode:       string(ds.Harness.Session.Mode),
		AgentCount: agentCount,
		StepCount:  stepCount,
	}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) toDomainDetail(d *domainmodel.RegisteredDomain) *viewmodel.DomainDetail {
	if d == nil {
		return nil
	}
	var harness any
	var spec *domain.DomainSpec
	if d.Spec != nil {
		harness = d.Spec.Harness
		spec = d.Spec
	}
	return &viewmodel.DomainDetail{
		ID:          d.ID,
		Version:     d.Version,
		Description: d.Description,
		Harness:     harness,
		Spec:        spec,
		Status:      "active",
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

// hook is a small internal wrapper around obs.HookFunc for nil-safe logging.
func (h *Handler) hook(ctx context.Context, name string, fields ...zap.Field) func() {
	if h.Logger == nil {
		return func() {}
	}
	fields = append(fields, obs.FieldsFromContext(ctx)...)
	return obs.HookFunc(ctx, name, h.Logger, fields...)
}

// ErrDomainExists is re-exported from commands so HTTP/gRPC can compare directly.
var ErrDomainExists = commands.ErrDomainExists

// IsDomainNotFound reports whether err is a domain-not-found error.
func IsDomainNotFound(err error) bool {
	return errors.Is(err, domainmodel.ErrDomainNotFound)
}

// IsDomainExists reports whether err is a domain-exists error.
func IsDomainExists(err error) bool {
	return errors.Is(err, domainmodel.ErrDomainExists) || errors.Is(err, ErrDomainExists)
}

// IsInvalidSpec reports whether err is an invalid-spec error.
func IsInvalidSpec(err error) bool {
	return errors.Is(err, domainmodel.ErrInvalidSpec)
}

// IsIDMismatch reports whether err is an ID-mismatch error.
func IsIDMismatch(err error) bool {
	return errors.Is(err, commands.ErrIDMismatch)
}
