// Package model defines the Worker aggregate for the HnsX control plane.
//
// A Worker represents a single Python Runtime Worker process that registers
// with the control plane, heartbeats periodically, and pulls sessions to
// execute. The control plane owns the worker lifecycle and session assignment.
package model

import (
	"errors"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// State represents the lifecycle of a worker from the control plane's view.
type State string

// Worker lifecycle states.
const (
	StateRegistered State = "registered"
	StateHealthy    State = "healthy"
	StateStale      State = "stale"
	StateEvicted    State = "evicted"
)

// Worker is the aggregate root for one runtime worker.
type Worker struct {
	ID               string
	Info             *pb.WorkerInfo
	LastSeen         time.Time
	State            State
	AssignedSessions []string
}

// IsHealthy reports whether the worker has heartbeated recently.
func (w *Worker) IsHealthy(maxAge time.Duration, now time.Time) bool {
	if w == nil {
		return false
	}
	return now.Sub(w.LastSeen) < maxAge
}

// Common errors returned by the worker service and repository.
var (
	ErrWorkerNotFound = errors.New("worker: not found")
	ErrWorkerExists   = errors.New("worker: already exists")
)
