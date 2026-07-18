package postgres

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
	"gorm.io/gorm"
)

// ErrEvalSetNotFound / ErrEvalRunNotFound.
var (
	ErrEvalSetNotFound = errors.New("eval set: not found")
	ErrEvalRunNotFound = errors.New("eval run: not found")
)

// EvalSetRepo implements eval.EvalSetRepo against Postgres via GORM.
type EvalSetRepo struct{ db *DB }

func NewEvalSetRepo(db *DB) *EvalSetRepo { return &EvalSetRepo{db: db} }

func (r *EvalSetRepo) Create(ctx context.Context, e *eval.EvalSet) error {
	if err := e.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *EvalSetRepo) Get(ctx context.Context, id string) (*eval.EvalSet, error) {
	var e eval.EvalSet
	err := r.db.WithContext(ctx).First(&e, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrEvalSetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EvalSetRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]*eval.EvalSet, error) {
	var out []*eval.EvalSet
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *EvalSetRepo) Update(ctx context.Context, e *eval.EvalSet) error {
	if err := e.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&eval.EvalSet{}).
		Where("id = ?", e.ID).
		Updates(map[string]any{
			"name":        e.Name,
			"description": e.Description,
			"cases":       e.Cases,
			"version":     e.Version,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrEvalSetNotFound
	}
	return nil
}

func (r *EvalSetRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&eval.EvalSet{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrEvalSetNotFound
	}
	return nil
}

var _ eval.EvalSetRepo = (*EvalSetRepo)(nil)

// EvalRunRepo implements eval.RunRepo against Postgres via GORM.
type EvalRunRepo struct{ db *DB }

func NewEvalRunRepo(db *DB) *EvalRunRepo { return &EvalRunRepo{db: db} }

func (r *EvalRunRepo) Create(ctx context.Context, run *eval.Run) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *EvalRunRepo) Get(ctx context.Context, id string) (*eval.Run, error) {
	var run eval.Run
	err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrEvalRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *EvalRunRepo) ListByEvalSet(ctx context.Context, evalSetID string, limit int) ([]*eval.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []*eval.Run
	err := r.db.WithContext(ctx).
		Where("eval_set_id = ?", evalSetID).
		Order("started_at DESC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *EvalRunRepo) Update(ctx context.Context, run *eval.Run) error {
	res := r.db.WithContext(ctx).
		Model(&eval.Run{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":       string(run.Status),
			"total_score":  run.TotalScore,
			"results":      run.Results,
			"completed_at": run.CompletedAt,
			"error":        run.Error,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrEvalRunNotFound
	}
	return nil
}

var _ eval.RunRepo = (*EvalRunRepo)(nil)