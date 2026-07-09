// Package worker holds the server-side data structures for tracking the
// Python worker pool: which workers are registered, what they can do, and
// which sessions are queued for them to pick up.
//
// Concurrency model: all exported methods are safe for concurrent use.
// The package keeps state in memory; persistence (to Postgres) is layered
// on top via the existing `runtimes` table but is not required for the
// Ray-style control plane to make scheduling decisions in V1.1.
package worker

import (
	"errors"
	"sync"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// WorkerRecord is the in-memory state of a single worker.
type WorkerRecord struct {
	Info     *pb.WorkerInfo
	LastSeen time.Time
	// Inbound is the queue of Cancel/Drain events to push down the
	// worker's StreamChannel. Buffered so the producer never blocks;
	// full queue means the worker is too slow — drop the oldest and
	// rely on a future heartbeat-driven drain to catch up.
	Inbound chan *pb.StreamChannelResponse
}

// Snapshot is a read-only view of a worker's runtime state.
type Snapshot struct {
	WorkerID    string
	Info        *pb.WorkerInfo
	LastSeen    time.Time
	AgeSeconds  float64
	Healthy     bool
}

// ErrUnknownWorker is returned by Heartbeat / Get when the worker_id has
// never been registered (or was evicted).
var ErrUnknownWorker = errors.New("worker: unknown worker_id")

// Registry tracks all live workers and their resource views.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*WorkerRecord
	now     func() time.Time
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		workers: map[string]*WorkerRecord{},
		now:     time.Now,
	}
}

// SetClock replaces the clock (test-only).
func (r *Registry) SetClock(fn func() time.Time) { r.now = fn }

// Register records a worker's WorkerInfo and returns a canonical worker_id.
// If ``info.WorkerId`` is empty a fresh id is minted from the current
// nanosecond clock + a short random suffix; otherwise the supplied id is
// reused (idempotent re-registration).
func (r *Registry) Register(info *pb.WorkerInfo) (string, error) {
	if info == nil {
		return "", errors.New("worker: nil WorkerInfo")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	wid := info.GetWorkerId()
	if wid == "" {
		wid = mintWorkerID()
		info.WorkerId = wid
	}

	if existing, ok := r.workers[wid]; ok {
		// Re-registration: update info + last-seen, keep the Inbound
		// channel (the previous StreamChannel may still be alive).
		existing.Info = info
		existing.LastSeen = r.now()
		return wid, nil
	}

	r.workers[wid] = &WorkerRecord{
		Info:     info,
		LastSeen: r.now(),
		Inbound:  make(chan *pb.StreamChannelResponse, 32),
	}
	return wid, nil
}

// Heartbeat updates the worker's last-seen timestamp and applies the
// reported resource view. Returns ErrUnknownWorker if the worker_id has
// not been registered (or has been evicted).
func (r *Registry) Heartbeat(workerID string, req *pb.HeartbeatRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.workers[workerID]
	if !ok {
		return ErrUnknownWorker
	}
	rec.LastSeen = r.now()
	if req != nil && req.Usage != nil {
		// Capacity upper bound is the worker's static declaration; the
		// dynamic view lives in ResourceUsage.
		if rec.Info.Capacity == nil {
			rec.Info.Capacity = &pb.ResourceCapacity{}
		}
		// Don't overwrite providers/models from the heartbeat; only the
		// resource usage moves.
	}
	return nil
}

// Get returns a snapshot of the named worker, or false.
func (r *Registry) Get(workerID string) (Snapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.workers[workerID]
	if !ok {
		return Snapshot{}, false
	}
	return Snapshot{
		WorkerID:   workerID,
		Info:       rec.Info,
		LastSeen:   rec.LastSeen,
		AgeSeconds: r.now().Sub(rec.LastSeen).Seconds(),
		Healthy:    r.now().Sub(rec.LastSeen) < 30*time.Second,
	}, true
}

// List returns a snapshot of every live worker.
func (r *Registry) List() []Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Snapshot, 0, len(r.workers))
	for id, rec := range r.workers {
		out = append(out, Snapshot{
			WorkerID:   id,
			Info:       rec.Info,
			LastSeen:   rec.LastSeen,
			AgeSeconds: r.now().Sub(rec.LastSeen).Seconds(),
			Healthy:    r.now().Sub(rec.LastSeen) < 30*time.Second,
		})
	}
	return out
}

// Inbound returns the cancel/drain push channel for the named worker, or
// nil if the worker_id is unknown. Producers (e.g. cancel APIs) call
// ``SendInbound(workerID, event)`` which is non-blocking; if the buffer
// is full the event is dropped and the caller can fall back to
// best-effort status updates.
func (r *Registry) Inbound(workerID string) chan *pb.StreamChannelResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.workers[workerID]
	if !ok {
		return nil
	}
	return rec.Inbound
}

// SendCancel publishes a CancelSessionCommand to the named worker's
// inbound queue. Non-blocking; returns false if the queue was full or
// the worker is unknown.
func (r *Registry) SendCancel(workerID, sessionID, reason string, deadlineMs int64) bool {
	ch := r.Inbound(workerID)
	if ch == nil {
		return false
	}
	evt := &pb.StreamChannelResponse{
		CorrelationId: sessionID,
		Payload: &pb.StreamChannelResponse_Cancel{
			Cancel: &pb.CancelSessionCommand{
				SessionId:  sessionID,
				Reason:     reason,
				DeadlineMs: deadlineMs,
			},
		},
	}
	select {
	case ch <- evt:
		return true
	default:
		return false
	}
}

// EvictStale removes workers whose last heartbeat is older than ``maxAge``.
// Returns the list of evicted worker_ids so callers can log / re-queue
// their in-flight sessions.
func (r *Registry) EvictStale(maxAge time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := r.now().Add(-maxAge)
	var evicted []string
	for id, rec := range r.workers {
		if rec.LastSeen.Before(cutoff) {
			close(rec.Inbound) // signal any open stream
			delete(r.workers, id)
			evicted = append(evicted, id)
		}
	}
	return evicted
}

// Len returns the number of registered workers (including stale).
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workers)
}

func mintWorkerID() string {
	return "w-" + time.Now().UTC().Format("20060102T150405.000000000")
}
