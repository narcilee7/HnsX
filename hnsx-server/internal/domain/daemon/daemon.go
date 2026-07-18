// Package daemon defines the Daemon aggregate.
//
// A Daemon is a long-running process (hnsxd daemon) registered against a
// workspace. It owns a set of runtimes (one per available CLI backend)
// and reports heartbeats so the control plane knows which agents it can
// currently execute.
package daemon

import (
	"context"
	"errors"
	"time"
)

// Status tracks the heartbeat-derived health of a daemon.
type Status string

const (
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
	StatusStale   Status = "stale" // last heartbeat > 30s ago
)

// Daemon is the aggregate root.
type Daemon struct {
	ID            string
	WorkspaceID   string
	Name          string
	Platform      string  // darwin | linux | windows
	OS            string  // normalized OS string
	Version       string  // hnsxd version
	Status        Status
	LastHeartbeat time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Validate enforces invariants.
func (d *Daemon) Validate() error {
	if d.WorkspaceID == "" {
		return errors.New("daemon: workspace_id is required")
	}
	if d.Name == "" {
		return errors.New("daemon: name is required")
	}
	return nil
}

// Repo is the persistence port.
type Repo interface {
	Register(ctx context.Context, d *Daemon) error
	Get(ctx context.Context, id string) (*Daemon, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Daemon, error)
	Heartbeat(ctx context.Context, id string, when time.Time) error
	UpdateStatus(ctx context.Context, id string, status Status) error
	Delete(ctx context.Context, id string) error
	MarkStale(ctx context.Context, cutoff time.Time) ([]string, error)
}