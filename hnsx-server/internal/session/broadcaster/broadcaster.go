// Package broadcaster implements a per-session pub/sub for observations.
package broadcaster

import (
	"context"
	"sync"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// Broadcaster is a per-session pub/sub for domain.Observation values.
//
// Subscribers register via Subscribe(); each receives:
//   - a replay buffer of all events published so far (so SSE clients that
//     connect after the session started can see history),
//   - followed by a live buffered channel of future events.
//
// Closed either by Close() or by the parent session ending.
type Broadcaster struct {
	mu        sync.Mutex
	subs      map[chan domain.Observation]struct{}
	closed    bool
	buffer    []domain.Observation
	bufferCap int
}

// NewBroadcaster creates an empty broadcaster with the default replay buffer
// (256 observations). Use WithBufferCap to adjust if your workload is bursty.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: map[chan domain.Observation]struct{}{}, bufferCap: 256}
}

// WithBufferCap sets the maximum number of recent observations retained for
// replay (defaults to 256).
func (b *Broadcaster) WithBufferCap(n int) *Broadcaster {
	if n > 0 {
		b.bufferCap = n
	}
	return b
}

// Subscribe registers a new observer. The returned channel is buffered with
// cap=64; if it fills the publisher blocks (slow consumer policy: wait).
// Replay events buffered before subscription are pre-filled into the channel
// before live events start streaming.
func (b *Broadcaster) Subscribe() (<-chan domain.Observation, func()) {
	ch := make(chan domain.Observation, 256)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	// Snapshot the replay buffer.
	replay := make([]domain.Observation, len(b.buffer))
	copy(replay, b.buffer)
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	// Drain replay into the channel without blocking on a closed state.
	go func() {
		for _, ev := range replay {
			ch <- ev
		}
	}()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; !ok {
			return
		}
		delete(b.subs, ch)
		close(ch)
	}
	return ch, cancel
}

// Publish records the observation in the replay buffer, then fans it out to
// every live subscriber. Returns ctx.Err() if the context is canceled mid-flight.
func (b *Broadcaster) Publish(ctx context.Context, obs domain.Observation) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	// Append to bounded buffer.
	b.buffer = append(b.buffer, obs)
	if len(b.buffer) > b.bufferCap {
		// Drop oldest entries to keep within cap.
		copy(b.buffer, b.buffer[len(b.buffer)-b.bufferCap:])
		b.buffer = b.buffer[:b.bufferCap]
	}
	subs := make([]chan domain.Observation, 0, len(b.subs))
	for ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- obs:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Close unsubscribes all observers.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}

// SubscriberCount returns the number of active observers. Useful for tests.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
