package postgres

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"gorm.io/gorm"
)

// ErrApprovalNotFound is returned when an approval lookup misses.
var ErrApprovalNotFound = errors.New("approval: not found")

// ApprovalRepo implements approval.Repo against Postgres via GORM.
type ApprovalRepo struct{ db *DB }

func NewApprovalRepo(db *DB) *ApprovalRepo { return &ApprovalRepo{db: db} }

func (r *ApprovalRepo) Create(ctx context.Context, a *approval.Approval) error {
	if err := a.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *ApprovalRepo) Get(ctx context.Context, id string) (*approval.Approval, error) {
	var a approval.Approval
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *ApprovalRepo) ListByIssue(ctx context.Context, issueID string) ([]*approval.Approval, error) {
	var out []*approval.Approval
	err := r.db.WithContext(ctx).
		Where("issue_id = ?", issueID).
		Order("requested_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ApprovalRepo) Update(ctx context.Context, a *approval.Approval) error {
	res := r.db.WithContext(ctx).
		Model(&approval.Approval{}).
		Where("id = ?", a.ID).
		Updates(map[string]any{
			"status":     string(a.Status),
			"decided_at": a.DecidedAt,
			"decided_by": a.DecidedBy,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrApprovalNotFound
	}
	return nil
}

func (r *ApprovalRepo) ListPending(ctx context.Context, workspaceID string) ([]*approval.Approval, error) {
	var out []*approval.Approval
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Where("status = ?", string(approval.StatusPending)).
		Order("requested_at ASC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

var _ approval.Repo = (*ApprovalRepo)(nil)