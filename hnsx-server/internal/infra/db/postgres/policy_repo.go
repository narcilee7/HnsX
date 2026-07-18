package postgres

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	"gorm.io/gorm"
)

// PolicyRepo implements policy.Repo against Postgres via GORM.
type PolicyRepo struct{ db *DB }

func NewPolicyRepo(db *DB) *PolicyRepo { return &PolicyRepo{db: db} }

func (r *PolicyRepo) Create(ctx context.Context, p *policy.Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *PolicyRepo) Get(ctx context.Context, id string) (*policy.Policy, error) {
	var p policy.Policy
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, policy.ErrPolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PolicyRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]*policy.Policy, error) {
	var out []*policy.Policy
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PolicyRepo) Update(ctx context.Context, p *policy.Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&policy.Policy{}).
		Where("id = ?", p.ID).
		Updates(map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"rules":       p.Rules,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return policy.ErrPolicyNotFound
	}
	return nil
}

func (r *PolicyRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&policy.Policy{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return policy.ErrPolicyNotFound
	}
	return nil
}

var _ policy.Repo = (*PolicyRepo)(nil)