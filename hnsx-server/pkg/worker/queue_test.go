package worker

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestQueue_EnqueueDequeue_FIFO(t *testing.T) {
	q := NewSessionQueue()
	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	q.Enqueue(&SessionRequest{SessionID: "s2", DomainID: "d"})
	q.Enqueue(&SessionRequest{SessionID: "s3", DomainID: "d"})

	if got, ok := q.Dequeue(context.Background(), nil); !ok || got.SessionID != "s1" {
		t.Fatalf("first dequeue = %v, %v", got, ok)
	}
	if got, ok := q.Dequeue(context.Background(), nil); !ok || got.SessionID != "s2" {
		t.Fatalf("second dequeue = %v, %v", got, ok)
	}
	if got, ok := q.Dequeue(context.Background(), nil); !ok || got.SessionID != "s3" {
		t.Fatalf("third dequeue = %v, %v", got, ok)
	}
}

func TestQueue_Dequeue_BlocksUntilAvailable(t *testing.T) {
	q := NewSessionQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	var got *SessionRequest
	var ok bool
	go func() {
		defer wg.Done()
		got, ok = q.Dequeue(ctx, nil)
	}()

	// Give the goroutine a moment to block.
	time.Sleep(20 * time.Millisecond)
	q.Enqueue(&SessionRequest{SessionID: "s-late", DomainID: "d"})
	wg.Wait()
	if !ok || got.SessionID != "s-late" {
		t.Fatalf("dequeue = %v, %v", got, ok)
	}
}

func TestQueue_Dequeue_CancelReturnsEmpty(t *testing.T) {
	q := NewSessionQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, ok := q.Dequeue(ctx, nil)
	elapsed := time.Since(start)
	if ok || got != nil {
		t.Fatalf("expected empty result, got %v, %v", got, ok)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("Dequeue should have returned within ~100ms, took %v", elapsed)
	}
}

func TestQueue_CapabilityMatch(t *testing.T) {
	q := NewSessionQueue()
	q.Enqueue(&SessionRequest{
		SessionID:           "s-anthropic",
		DomainID:            "d",
		RequiredCapabilities: []string{"provider:anthropic"},
	})
	q.Enqueue(&SessionRequest{
		SessionID:           "s-openai",
		DomainID:            "d",
		RequiredCapabilities: []string{"provider:openai"},
	})

	// A worker that only handles anthropic should get s-anthropic.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	got, ok := q.Dequeue(ctx, []string{"provider:anthropic"})
	if !ok || got.SessionID != "s-anthropic" {
		t.Fatalf("expected s-anthropic, got %v, %v", got, ok)
	}

	// Next: a worker that handles only openai should get s-openai.
	got, ok = q.Dequeue(ctx, []string{"provider:openai"})
	if !ok || got.SessionID != "s-openai" {
		t.Fatalf("expected s-openai, got %v, %v", got, ok)
	}

	// No more sessions, no match.
	_, ok = q.Dequeue(ctx, []string{"provider:anthropic"})
	if ok {
		t.Fatalf("expected empty after both dequeued")
	}
}

func TestQueue_Remove(t *testing.T) {
	q := NewSessionQueue()
	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	q.Enqueue(&SessionRequest{SessionID: "s2", DomainID: "d"})
	q.Remove("s1")
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
	got, ok := q.Dequeue(context.Background(), nil)
	if !ok || got.SessionID != "s2" {
		t.Fatalf("expected s2, got %v, %v", got, ok)
	}
}

func TestQueue_EnqueueIdempotent(t *testing.T) {
	q := NewSessionQueue()
	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
}
