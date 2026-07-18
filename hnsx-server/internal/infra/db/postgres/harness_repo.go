package postgres

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
	"gorm.io/gorm"
)

// HarnessRepo implements harness.Repo against Postgres via GORM.
type HarnessRepo struct{ db *DB }

func NewHarnessRepo(db *DB) *HarnessRepo { return &HarnessRepo{db: db} }

func (r *HarnessRepo) Create(ctx context.Context, h *harness.Harness) error {
	if err := h.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *HarnessRepo) Get(ctx context.Context, id string) (*harness.Harness, error) {
	var h harness.Harness
	err := r.db.WithContext(ctx).First(&h, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, harness.ErrHarnessNotFound
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HarnessRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]*harness.Harness, error) {
	var out []*harness.Harness
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *HarnessRepo) Update(ctx context.Context, h *harness.Harness) error {
	if err := h.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&harness.Harness{}).
		Where("id = ?", h.ID).
		Updates(map[string]any{
			"name":        h.Name,
			"description": h.Description,
			"prompts":     h.Prompts,
			"skills":      h.Skills,
			"tools":       h.Tools,
			"policy_id":   h.PolicyID,
			"eval_set_id": h.EvalSetID,
			"version":     h.Version,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return harness.ErrHarnessNotFound
	}
	return nil
}

func (r *HarnessRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&harness.Harness{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return harness.ErrHarnessNotFound
	}
	return nil
}

var _ harness.Repo = (*HarnessRepo)(nil)