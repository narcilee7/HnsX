// Package service implements the worker application use cases.
//
// It owns worker registration, heartbeat, eviction, session assignment,
// and the session scheduling queue. The actual Agent execution is delegated
// to Python Runtime Workers via the gRPC control plane.
package service

import (
	"context"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/worker"
	"github.com/hnsx-io/hnsx/server/internal/worker/model"
	"github.com/hnsx-io/hnsx/server/internal/worker/repository"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// Service implements the worker application use cases.
type Service struct {
	repo  repository.Repository
	reg   *worker.Registry
	queue worker.SessionQueue
}

// NewService constructs a Service backed by the supplied repository and an
// in-memory session queue. Use NewServiceWithQueue to inject a different
// queue implementation (e.g. RedisSessionQueue for multi-instance Control
// Plane).
func NewService(repo repository.Repository) *Service {
	return NewServiceWithQueue(repo, worker.NewSessionQueue())
}

// NewServiceWithQueue constructs a Service with an explicit session queue.
func NewServiceWithQueue(repo repository.Repository, q worker.SessionQueue) *Service {
	return &Service{
		repo:  repo,
		reg:   worker.NewRegistry(),
		queue: q,
	}
}

// WithQueue replaces the default session queue. Used by tests and main.
func (s *Service) WithQueue(q worker.SessionQueue) *Service {
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
func (s *Service) Queue() worker.SessionQueue { return s.queue }

// InboundChannel returns the cancel/drain push channel for the named worker,
// or nil if the worker_id is unknown.
func (s *Service) InboundChannel(workerID string) <-chan *pb.StreamChannelResponse {
	if s.reg == nil {
		return nil
	}
	return s.reg.Inbound(workerID)
}

// SessionsForWorker returns the session IDs currently assigned to a worker.
func (s *Service) SessionsForWorker(workerID string) []string {
	if s.reg == nil {
		return nil
	}
	return s.reg.SessionsForWorker(workerID)
}

// RemoveWorkerSessions drops all session assignments for a worker without
// evicting the worker itself.
func (s *Service) RemoveWorkerSessions(workerID string) {
	if s.reg == nil {
		return
	}
	s.reg.RemoveWorkerSessions(workerID)
}

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

// ServiceStats is a point-in-time snapshot of the worker pool.
type ServiceStats struct {
	Workers           int
	HealthyWorkers    int
	QueueLen          int
	ActiveAssignments int
}

// Stats returns a snapshot of the worker pool and queue.
func (s *Service) Stats() ServiceStats {
	st := ServiceStats{}
	if s.reg != nil {
		rs := s.reg.Stats()
		st.Workers = rs.Workers
		st.HealthyWorkers = rs.Healthy
		st.ActiveAssignments = rs.ActiveSessions
	}
	if s.queue != nil {
		st.QueueLen = s.queue.Len()
	}
	return st
}

// Close releases registry resources. Safe to call on a nil service.
func (s *Service) Close() {
	if s == nil {
		return
	}
	if s.reg != nil {
		s.reg.Close()
	}
}

// QueueLen returns the current pending session count.
func (s *Service) QueueLen() int {
	if s.queue == nil {
		return 0
	}
	return s.queue.Len()
}
