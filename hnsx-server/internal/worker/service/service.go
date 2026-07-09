// Package service implements the worker application use cases.
//
// It owns worker registration, heartbeat, eviction, session assignment,
// and the session scheduling queue. The actual Agent execution is delegated
// to Python Runtime Workers via the gRPC control plane.
package service

import (
	"context"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	"github.com/hnsx-io/hnsx/server/internal/worker/model"
	"github.com/hnsx-io/hnsx/server/internal/worker/repository"
)

// Service implements the worker application use cases.
type Service struct {
	repo  repository.Repository
	reg   *worker.Registry
	queue *worker.SessionQueue
}

// NewService constructs a Service backed by the supplied repository.
// It also owns the in-memory registry and session queue used by the gRPC
// control plane. Pass nil for the queue if worker pooling is disabled.
func NewService(repo repository.Repository) *Service {
	return &Service{
		repo:  repo,
		reg:   worker.NewRegistry(),
		queue: worker.NewSessionQueue(),
	}
}

// WithQueue replaces the default session queue. Used by tests.
func (s *Service) WithQueue(q *worker.SessionQueue) *Service {
	s.queue = q
	return s
}

// WithRegistry replaces the default registry. Used by tests.
func (s *Service) WithRegistry(r *worker.Registry) *Service {
	s.reg = r
	return s
}

// Registry returns the underlying in-memory registry.
// Infrastructure adapters (gRPC scheduler service) still need direct access
// until the registry is fully encapsulated behind this service.
func (s *Service) Registry() *worker.Registry { return s.reg }

// Queue returns the underlying session queue.
func (s *Service) Queue() *worker.SessionQueue { return s.queue }

// Register records a worker's WorkerInfo and returns a canonical worker_id.
func (s *Service) Register(info *pb.WorkerInfo) (string, error) {
	id, err := s.reg.Register(info)
	if err != nil {
		return "", err
	}
	// Mirror into repository for persistence / recovery.
	_ = s.repo.Save(&model.Worker{
		ID:       id,
		Info:     info,
		LastSeen: time.Now().UTC(),
		State:    model.StateRegistered,
	})
	return id, nil
}

// Heartbeat updates the worker's last-seen timestamp.
func (s *Service) Heartbeat(workerID string, req *pb.HeartbeatRequest) error {
	if err := s.reg.Heartbeat(workerID, req); err != nil {
		return err
	}
	w, err := s.repo.ByID(workerID)
	if err != nil {
		// Best-effort persistence mirror; don't fail heartbeat.
		return nil
	}
	w.LastSeen = time.Now().UTC()
	w.State = model.StateHealthy
	_ = s.repo.Save(w)
	return nil
}

// Get returns a snapshot of the named worker.
func (s *Service) Get(workerID string) (worker.Snapshot, bool) {
	return s.reg.Get(workerID)
}

// List returns every live worker.
func (s *Service) List() []worker.Snapshot {
	return s.reg.List()
}

// EvictStale removes workers whose last heartbeat is older than maxAge.
func (s *Service) EvictStale(maxAge time.Duration) []string {
	return s.reg.EvictStale(maxAge)
}

// AssignSession records that workerID is now responsible for sessionID.
func (s *Service) AssignSession(workerID, sessionID string) {
	s.reg.AssignSession(workerID, sessionID)
}

// UnassignSession removes the session-to-worker mapping.
func (s *Service) UnassignSession(sessionID string) {
	s.reg.UnassignSession(sessionID)
}

// SessionWorker returns the worker_id currently assigned to sessionID.
func (s *Service) SessionWorker(sessionID string) (string, bool) {
	return s.reg.SessionWorker(sessionID)
}

// SendCancel publishes a CancelSessionCommand to the worker's inbound queue.
func (s *Service) SendCancel(workerID, sessionID, reason string, deadlineMs int64) bool {
	return s.reg.SendCancel(workerID, sessionID, reason, deadlineMs)
}

// EnqueueSession adds a session request to the scheduling queue.
func (s *Service) EnqueueSession(req *worker.SessionRequest) {
	if s.queue == nil {
		return
	}
	s.queue.Enqueue(req)
}

// DequeueSession blocks until a session matching required capabilities is
// available or the context is cancelled.
func (s *Service) DequeueSession(ctx context.Context, required []string) (*worker.SessionRequest, bool) {
	if s.queue == nil {
		return nil, false
	}
	return s.queue.Dequeue(ctx, required)
}

// QueueLen returns the current pending session count.
func (s *Service) QueueLen() int {
	if s.queue == nil {
		return 0
	}
	return s.queue.Len()
}
