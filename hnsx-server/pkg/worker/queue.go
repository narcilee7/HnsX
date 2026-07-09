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

// SessionQueue is an in-memory FIFO with capability matching and
// long-poll support. PullSession blocks until either a matching session
// is available or the caller's context is cancelled (e.g. the worker
// times out the long-poll).
type SessionQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []*SessionRequest
	byID    map[string]*SessionRequest
}

// NewSessionQueue constructs an empty SessionQueue.
func NewSessionQueue() *SessionQueue {
	q := &SessionQueue{byID: map[string]*SessionRequest{}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Enqueue adds a session to the back of the queue. Wakes one blocked
// Dequeue caller, if any. If a session with the same SessionID is
// already queued, the call is a no-op (idempotent).
func (q *SessionQueue) Enqueue(req *SessionRequest) {
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
func (q *SessionQueue) Dequeue(ctx context.Context, required []string) (*SessionRequest, bool) {
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
func (q *SessionQueue) Remove(id string) {
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
func (q *SessionQueue) Len() int {
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
