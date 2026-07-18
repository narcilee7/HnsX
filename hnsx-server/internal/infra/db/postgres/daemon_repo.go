package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
	"gorm.io/gorm"
)

// daemon.ErrDaemonNotFound is returned when a daemon lookup misses.

// DaemonRepo implements daemon.Repo against Postgres via GORM.
type DaemonRepo struct{ db *DB }

func NewDaemonRepo(db *DB) *DaemonRepo { return &DaemonRepo{db: db} }

func (r *DaemonRepo) Register(ctx context.Context, d *daemon.Daemon) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if d.LastHeartbeat.IsZero() {
		d.LastHeartbeat = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *DaemonRepo) Get(ctx context.Context, id string) (*daemon.Daemon, error) {
	var d daemon.Daemon
	err := r.db.WithContext(ctx).First(&d, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, daemon.ErrDaemonNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DaemonRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]*daemon.Daemon, error) {
	var out []*daemon.Daemon
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("last_heartbeat DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DaemonRepo) Heartbeat(ctx context.Context, id string, when time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&daemon.Daemon{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_heartbeat": when,
			"status":         string(daemon.StatusOnline),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return daemon.ErrDaemonNotFound
	}
	return nil
}

func (r *DaemonRepo) UpdateStatus(ctx context.Context, id string, status daemon.Status) error {
	res := r.db.WithContext(ctx).
		Model(&daemon.Daemon{}).
		Where("id = ?", id).
		Update("status", string(status))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return daemon.ErrDaemonNotFound
	}
	return nil
}

func (r *DaemonRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&daemon.Daemon{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return daemon.ErrDaemonNotFound
	}
	return nil
}

func (r *DaemonRepo) MarkStale(ctx context.Context, cutoff time.Time) ([]string, error) {
	// Returns IDs of daemons transitioned online -> stale by this sweep.
	res := r.db.WithContext(ctx).
		Model(&daemon.Daemon{}).
		Where("status = ?", string(daemon.StatusOnline)).
		Where("last_heartbeat < ?", cutoff).
		Update("status", string(daemon.StatusStale))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	// Fetch the IDs we just flipped so callers can log / re-queue.
	var stale []daemon.Daemon
	if err := r.db.WithContext(ctx).
		Where("status = ?", string(daemon.StatusStale)).
		Where("last_heartbeat < ?", cutoff).
		Find(&stale).Error; err != nil {
		return nil, err
	}
	ids := make([]string, len(stale))
	for i, d := range stale {
		ids[i] = d.ID
	}
	return ids, nil
}

var _ daemon.Repo = (*DaemonRepo)(nil)