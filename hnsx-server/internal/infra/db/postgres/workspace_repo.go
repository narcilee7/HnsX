package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrWorkspaceNotFound is returned when a workspace lookup misses. The
// service layer translates this into a 404 at the HTTP boundary.
var ErrWorkspaceNotFound = errors.New("workspace: not found")

// WorkspaceRepo implements workspace.Repo against Postgres via GORM.
type WorkspaceRepo struct{ db *DB }

// NewWorkspaceRepo binds the repo to a shared *DB. The *DB is goroutine-
// safe; one repo per resource is shared across the app.
func NewWorkspaceRepo(db *DB) *WorkspaceRepo { return &WorkspaceRepo{db: db} }

func (r *WorkspaceRepo) Create(ctx context.Context, w *workspace.Workspace) error {
	if err := w.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *WorkspaceRepo) Get(ctx context.Context, id string) (*workspace.Workspace, error) {
	var w workspace.Workspace
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WorkspaceRepo) GetBySlug(ctx context.Context, slug string) (*workspace.Workspace, error) {
	var w workspace.Workspace
	err := r.db.WithContext(ctx).First(&w, "slug = ?", slug).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WorkspaceRepo) List(ctx context.Context, f workspace.ListFilter) ([]*workspace.Workspace, error) {
	q := r.db.WithContext(ctx).Model(&workspace.Workspace{})
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var out []*workspace.Workspace
	if err := q.Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *WorkspaceRepo) Update(ctx context.Context, w *workspace.Workspace) error {
	if err := w.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&workspace.Workspace{}).
		Where("id = ?", w.ID).
		Clauses(clause.Returning{}).
		Updates(map[string]any{
			"name":        w.Name,
			"description": w.Description,
			"context":     w.Context,
			"settings":    w.Settings,
			"status":      string(w.Status),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

func (r *WorkspaceRepo) Archive(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).
		Model(&workspace.Workspace{}).
		Where("id = ?", id).
		Update("status", string(workspace.StatusArchived))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

func (r *WorkspaceRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&workspace.Workspace{}, "id = ?", id)
	if res.Error != nil {
		// FK violation => workspace still owns agents/issues/etc.
		if strings.Contains(res.Error.Error(), "foreign key") {
			return errors.New("workspace: cannot delete, still has child resources")
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// _ guards against accidental signature drift on the repo port.
var _ workspace.Repo = (*WorkspaceRepo)(nil)