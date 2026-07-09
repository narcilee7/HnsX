package worker

import (
	"context"
	"sync"
	"time"
)

// SessionRequest is the server-side view of a pending session that a
// worker can pull. It carries the same fields the server already has
// from the original /api/v1/sessions trigger; we just plumb them through
// to the worker.
type SessionRequest struct {
	SessionID            string
	DomainID             string
	DomainVersion        string
	DomainSpecJSON       string
	TriggerPayloadJSON   string
	TraceID              string
	RequiredCapabilities []string // e.g. ["provider:anthropic","model:claude-haiku-4-5"]
	EnqueuedAt           time.Time
	CorrelationID        string
}

// SessionQueue is the scheduler-side abstraction for pending sessions.
// Implementations may be in-memory (single-process / tests) or backed by
// Redis (multi-instance Control Plane).
type SessionQueue interface {
	// Enqueue adds a session to the queue. Implementations must be
	// idempotent: enqueuing the same SessionID twice is a no-op.
	Enqueue(req *SessionRequest)
	// Dequeue blocks until a session matching all required capabilities is
	// available or the context is cancelled. Returns (nil, false) on
	// cancellation.
	Dequeue(ctx context.Context, required []string) (*SessionRequest, bool)
	// Remove deletes a session by id (e.g. after Ack or Nack).
	Remove(id string)
	// Len returns the current pending count.
	Len() int
}

// MemorySessionQueue is an in-memory FIFO with capability matching and
// long-poll support. It is the default when no Redis backend is configured
// and is useful for tests or single-process deployments.
type MemorySessionQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []*SessionRequest
	byID    map[string]*SessionRequest
}

// NewSessionQueue constructs an empty in-memory SessionQueue.
func NewSessionQueue() SessionQueue {
	q := &MemorySessionQueue{byID: map[string]*SessionRequest{}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// NewMemorySessionQueue is an explicit alias of NewSessionQueue. Use it when
// the caller wants to document that an in-memory queue is intentional.
func NewMemorySessionQueue() SessionQueue { return NewSessionQueue() }

// Enqueue adds a session to the back of the queue. Wakes one blocked
// Dequeue caller, if any. If a session with the same SessionID is
// already queued, the call is a no-op (idempotent).
func (q *MemorySessionQueue) Enqueue(req *SessionRequest) {
	if req == nil || req.SessionID == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.byID[req.SessionID]; exists {
		return
	}
	if req.EnqueuedAt.IsZero() {
		req.EnqueuedAt = time.Now().UTC()
	}
	q.pending = append(q.pending, req)
	q.byID[req.SessionID] = req
	q.cond.Signal()
}

// Dequeue blocks until either a session matching “required“ is
// available or “ctx“ is cancelled. The match rule is "all required
// capabilities must be present in the request's RequiredCapabilities"
// (intersection is non-empty for every key). Returns (nil, false) on
// cancellation.
func (q *MemorySessionQueue) Dequeue(ctx context.Context, required []string) (*SessionRequest, bool) {
	// Watch for ctx cancellation while waiting.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.cond.Broadcast() // wake any waiters so they see the cancel
			q.mu.Unlock()
		case <-stop:
		}
	}()

	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		for i, s := range q.pending {
			if matches(s.RequiredCapabilities, required) {
				// pop
				q.pending = append(q.pending[:i], q.pending[i+1:]...)
				delete(q.byID, s.SessionID)
				return s, true
			}
		}
		if ctx.Err() != nil {
			return nil, false
		}
		q.cond.Wait()
	}
}

// Remove deletes a session by id (e.g. after Ack or Nack).
func (q *MemorySessionQueue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.byID[id]; !ok {
		return
	}
	for i, s := range q.pending {
		if s.SessionID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			break
		}
	}
	delete(q.byID, id)
}

// Len returns the current pending count.
func (q *MemorySessionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// matches: every required capability must appear in the offered
// RequiredCapabilities list. A session with no RequiredCapabilities
// matches every required filter (acts as "any worker can pick this up").
func matches(offered, required []string) bool {
	if len(required) == 0 {
		return true
	}
	have := make(map[string]struct{}, len(offered))
	for _, c := range offered {
		have[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			return false
		}
	}
	return true
}

var _ SessionQueue = (*MemorySessionQueue)(nil)
