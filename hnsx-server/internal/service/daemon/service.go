// Package daemon hosts the application-level orchestration for Daemon.
package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
)

// Service is the daemon application service.
type Service struct {
	repo daemon.Repo
}

// New wires a repo into a Service.
func New(repo daemon.Repo) *Service { return &Service{repo: repo} }

// RegisterInput is what the daemon sends when it first connects.
type RegisterInput struct {
	WorkspaceID string
	Name        string
	Platform    string
	OS          string
	Version     string
}

// Register assigns an ID + initial heartbeat and persists.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*daemon.Daemon, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, errors.New("daemon: workspace_id is required")
	}
	d := &daemon.Daemon{
		ID:            uuid.NewString(),
		WorkspaceID:   in.WorkspaceID,
		Name:          strings.TrimSpace(in.Name),
		Platform:      in.Platform,
		OS:            in.OS,
		Version:       in.Version,
		Status:        daemon.StatusOnline,
		LastHeartbeat: time.Now().UTC(),
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Register(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*daemon.Daemon, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns all daemons in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]*daemon.Daemon, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Heartbeat records that the daemon is still alive.
func (s *Service) Heartbeat(ctx context.Context, id string) error {
	return s.repo.Heartbeat(ctx, id, time.Now().UTC())
}

// UpdateStatus changes a daemon's status flag.
func (s *Service) UpdateStatus(ctx context.Context, id string, status daemon.Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

// Delete removes a daemon record. Called when the daemon explicitly
// unregisters or after the 7-day offline GC sweep.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// MarkStale sweeps daemons whose heartbeat is older than cutoff and
// transitions them online -> stale. Returns the IDs that flipped.
func (s *Service) MarkStale(ctx context.Context, cutoff time.Time) ([]string, error) {
	return s.repo.MarkStale(ctx, cutoff)
}