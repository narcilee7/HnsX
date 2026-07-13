package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"

	secmodel "github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// ListSecretsInput selects the secret list to return.
type ListSecretsInput struct{}

// GetSecretInput selects a single secret by name.
type GetSecretInput struct {
	Name string
}

// CreateSecretInput carries the plaintext secret to create.
type CreateSecretInput struct {
	Name        string
	Value       string
	Description string
	Kind        string
}

// UpdateSecretInput carries the replacement plaintext secret.
type UpdateSecretInput struct {
	Name        string
	Value       string
	Description string
	Kind        string
}

// DeleteSecretInput selects the secret to delete.
type DeleteSecretInput struct {
	Name string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

// ListSecretsOutput is a paginated list of secrets.
type ListSecretsOutput struct {
	Secrets viewmodel.SecretList
}

// GetSecretOutput is the detail view of a single secret.
type GetSecretOutput struct {
	Secret *viewmodel.SecretDetail
}

// CreateSecretOutput is the ack returned after creating a secret.
type CreateSecretOutput struct {
	Secret *viewmodel.SecretCreated
}

// UpdateSecretOutput is the ack returned after updating a secret.
type UpdateSecretOutput struct {
	Secret *viewmodel.SecretUpdated
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListSecrets returns metadata for every stored secret.
func (h *Handler) ListSecrets(ctx context.Context, in ListSecretsInput) (*ListSecretsOutput, error) {
	defer h.hook(ctx, "secret.list")()

	if h.App == nil || h.App.SecretService == nil {
		return nil, secmodel.ErrSecretNotFound
	}
	items, err := h.App.SecretService.List()
	if err != nil {
		return nil, err
	}

	out := make([]viewmodel.SecretListItem, 0, len(items))
	for _, it := range items {
		out = append(out, viewmodel.SecretListItem{
			Name:        it.Name,
			Description: it.Description,
			Kind:        it.Kind,
			Fingerprint: it.Fingerprint,
			CreatedAt:   it.CreatedAt,
			UpdatedAt:   it.UpdatedAt,
		})
	}

	return &ListSecretsOutput{Secrets: viewmodel.SecretList{
		Items:  out,
		Total:  len(out),
		Limit:  len(out),
		Offset: 0,
	}}, nil
}

// GetSecret returns a single secret by name.
func (h *Handler) GetSecret(ctx context.Context, in GetSecretInput) (*GetSecretOutput, error) {
	defer h.hook(ctx, "secret.get", zap.String("name", in.Name))()

	if h.App == nil || h.App.SecretService == nil {
		return nil, secmodel.ErrSecretNotFound
	}
	sec, err := h.App.SecretService.ByName(in.Name)
	if err != nil {
		return nil, err
	}
	return &GetSecretOutput{Secret: h.toSecretDetail(sec)}, nil
}

// CreateSecret persists a new encrypted secret.
func (h *Handler) CreateSecret(ctx context.Context, in CreateSecretInput) (*CreateSecretOutput, error) {
	defer h.hook(ctx, "secret.create", zap.String("name", in.Name))()

	if in.Name == "" || in.Value == "" {
		return nil, secmodel.ErrInvalidName
	}
	if h.App == nil || h.App.SecretService == nil {
		return nil, secmodel.ErrSecretNotFound
	}
	sec := &secmodel.Secret{
		Name:        in.Name,
		Description: in.Description,
		Kind:        in.Kind,
		PlainValue:  in.Value,
	}
	if err := h.App.SecretService.Save(sec); err != nil {
		return nil, err
	}
	return &CreateSecretOutput{Secret: &viewmodel.SecretCreated{
		Name:        sec.Name,
		Description: sec.Description,
		Kind:        sec.Kind,
		Fingerprint: sec.Fingerprint,
	}}, nil
}

// UpdateSecret replaces the value and metadata of an existing secret.
func (h *Handler) UpdateSecret(ctx context.Context, in UpdateSecretInput) (*UpdateSecretOutput, error) {
	defer h.hook(ctx, "secret.update", zap.String("name", in.Name))()

	if in.Name == "" {
		return nil, secmodel.ErrInvalidName
	}
	if in.Value == "" {
		return nil, secmodel.ErrInvalidName
	}
	if h.App == nil || h.App.SecretService == nil {
		return nil, secmodel.ErrSecretNotFound
	}
	sec := &secmodel.Secret{
		Name:        in.Name,
		Description: in.Description,
		Kind:        in.Kind,
		PlainValue:  in.Value,
	}
	if err := h.App.SecretService.Save(sec); err != nil {
		return nil, err
	}
	return &UpdateSecretOutput{Secret: &viewmodel.SecretUpdated{
		Name:        sec.Name,
		Description: sec.Description,
		Kind:        sec.Kind,
		Fingerprint: sec.Fingerprint,
	}}, nil
}

// DeleteSecret removes a secret by name.
func (h *Handler) DeleteSecret(ctx context.Context, in DeleteSecretInput) error {
	defer h.hook(ctx, "secret.delete", zap.String("name", in.Name))()

	if h.App == nil || h.App.SecretService == nil {
		return secmodel.ErrSecretNotFound
	}
	return h.App.SecretService.Delete(in.Name)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) toSecretDetail(sec *secmodel.Secret) *viewmodel.SecretDetail {
	if sec == nil {
		return nil
	}
	return &viewmodel.SecretDetail{
		Name:        sec.Name,
		Description: sec.Description,
		Kind:        sec.Kind,
		Fingerprint: sec.Fingerprint,
		CreatedAt:   sec.CreatedAt,
		UpdatedAt:   sec.UpdatedAt,
	}
}

// Secret errors re-exported for HTTP/gRPC comparability.
var (
	ErrSecretNotFound = secmodel.ErrSecretNotFound
	ErrInvalidName    = secmodel.ErrInvalidName
	ErrSecretExists   = secmodel.ErrSecretExists
)

// IsSecretNotFound reports whether err is a secret-not-found error.
func IsSecretNotFound(err error) bool {
	return errors.Is(err, secmodel.ErrSecretNotFound)
}

// IsSecretExists reports whether err is a secret-already-exists error.
func IsSecretExists(err error) bool {
	return errors.Is(err, secmodel.ErrSecretExists)
}

// IsInvalidSecretName reports whether err is an invalid-secret-name error.
func IsInvalidSecretName(err error) bool {
	return errors.Is(err, secmodel.ErrInvalidName)
}

// mapSecretError maps a secret service error to a stable viewmodel error.
func mapSecretError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case IsSecretNotFound(err):
		return ErrSecretNotFound
	case IsSecretExists(err):
		return ErrSecretExists
	case IsInvalidSecretName(err):
		return ErrInvalidName
	}
	return err
}
