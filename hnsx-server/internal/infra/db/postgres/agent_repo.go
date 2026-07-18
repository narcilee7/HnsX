package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	"gorm.io/gorm"
)

// agent.ErrAgentNotFound is returned when an agent lookup misses.

// AgentRepo implements agent.Repo against Postgres via GORM.
type AgentRepo struct{ db *DB }

func NewAgentRepo(db *DB) *AgentRepo { return &AgentRepo{db: db} }

func (r *AgentRepo) Create(ctx context.Context, a *agent.Agent) error {
	if err := a.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *AgentRepo) Get(ctx context.Context, id string) (*agent.Agent, error) {
	var a agent.Agent
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, agent.ErrAgentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AgentRepo) ListByWorkspace(ctx context.Context, workspaceID string, f agent.ListFilter) ([]*agent.Agent, error) {
	q := r.db.WithContext(ctx).Model(&agent.Agent{}).Where("workspace_id = ?", workspaceID)
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.Visibility != "" {
		q = q.Where("visibility = ?", string(f.Visibility))
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var out []*agent.Agent
	if err := q.Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *AgentRepo) Update(ctx context.Context, a *agent.Agent) error {
	if err := a.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&agent.Agent{}).
		Where("id = ?", a.ID).
		Updates(map[string]any{
			"name":                 a.Name,
			"description":          a.Description,
			"avatar_url":           a.AvatarURL,
			"runtime_mode":         string(a.RuntimeMode),
			"runtime_config":       a.RuntimeConfig,
			"visibility":           string(a.Visibility),
			"max_concurrent_tasks": a.MaxConcurrentTasks,
			"owner_id":             a.OwnerID,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

func (r *AgentRepo) UpdateStatus(ctx context.Context, id string, status agent.Status) error {
	res := r.db.WithContext(ctx).
		Model(&agent.Agent{}).
		Where("id = ?", id).
		Update("status", string(status))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

func (r *AgentRepo) Archive(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&agent.Agent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"archived_at": &now,
			"status":      string(agent.StatusOffline),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

func (r *AgentRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&agent.Agent{}, "id = ?", id)
	if res.Error != nil {
		if strings.Contains(res.Error.Error(), "foreign key") {
			return errors.New("agent: cannot delete, still referenced by issues or squads")
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

var _ agent.Repo = (*AgentRepo)(nil)