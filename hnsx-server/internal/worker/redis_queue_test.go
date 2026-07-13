package worker

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupRedisQueue(t *testing.T) (SessionQueue, func()) {
	t.Helper()
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	q := NewRedisSessionQueue(rdb, "test:queue")
	return q, func() {
		_ = rdb.Close()
		s.Close()
	}
}

func TestRedisQueue_EnqueueDequeue_FIFO(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

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

func TestRedisQueue_Dequeue_BlocksUntilAvailable(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var got *SessionRequest
	var ok bool
	go func() {
		defer close(done)
		got, ok = q.Dequeue(ctx, nil)
	}()

	time.Sleep(100 * time.Millisecond)
	q.Enqueue(&SessionRequest{SessionID: "s-late", DomainID: "d"})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Dequeue did not return in time")
	}
	if !ok || got.SessionID != "s-late" {
		t.Fatalf("dequeue = %v, %v", got, ok)
	}
}

func TestRedisQueue_Dequeue_CancelReturnsEmpty(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

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

func TestRedisQueue_CapabilityMatch(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	q.Enqueue(&SessionRequest{
		SessionID:            "s-anthropic",
		DomainID:             "d",
		RequiredCapabilities: []string{"provider:anthropic"},
	})
	q.Enqueue(&SessionRequest{
		SessionID:            "s-openai",
		DomainID:             "d",
		RequiredCapabilities: []string{"provider:openai"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	got, ok := q.Dequeue(ctx, []string{"provider:anthropic"})
	if !ok || got.SessionID != "s-anthropic" {
		t.Fatalf("expected s-anthropic, got %v, %v", got, ok)
	}

	got, ok = q.Dequeue(ctx, []string{"provider:openai"})
	if !ok || got.SessionID != "s-openai" {
		t.Fatalf("expected s-openai, got %v, %v", got, ok)
	}

	_, ok = q.Dequeue(ctx, []string{"provider:anthropic"})
	if ok {
		t.Fatalf("expected empty after both dequeued")
	}
}

func TestRedisQueue_Remove(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

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

func TestRedisQueue_Recover(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})

	err := q.Recover([]*SessionRequest{
		{SessionID: "s1", DomainID: "d"}, // duplicate
		{SessionID: "s2", DomainID: "d"},
		{SessionID: "s3", DomainID: "d"},
	})
	if err != nil {
		t.Fatalf("Recover error: %v", err)
	}
	if q.Len() != 3 {
		t.Fatalf("Len = %d, want 3", q.Len())
	}

	ctx := context.Background()
	for _, want := range []string{"s1", "s2", "s3"} {
		got, ok := q.Dequeue(ctx, nil)
		if !ok || got.SessionID != want {
			t.Fatalf("dequeue = %v, %v, want %s", got, ok, want)
		}
	}
}

func TestRedisQueue_EnqueueIdempotent(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	q.Enqueue(&SessionRequest{SessionID: "s1", DomainID: "d"})
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
}

func TestRedisQueue_PreservesFields(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	in := &SessionRequest{
		SessionID:            "s-fields",
		DomainID:             "domain-1",
		DomainVersion:        "v2",
		DomainSpecJSON:       `{"id":"domain-1"}`,
		TriggerPayloadJSON:   `{"input":"hello"}`,
		TraceID:              "trace-abc",
		RequiredCapabilities: []string{"provider:anthropic", "model:claude"},
		CorrelationID:        "corr-xyz",
	}
	q.Enqueue(in)

	got, ok := q.Dequeue(context.Background(), []string{"provider:anthropic"})
	if !ok {
		t.Fatal("expected dequeue to succeed")
	}
	if got.SessionID != in.SessionID ||
		got.DomainID != in.DomainID ||
		got.DomainVersion != in.DomainVersion ||
		got.DomainSpecJSON != in.DomainSpecJSON ||
		got.TriggerPayloadJSON != in.TriggerPayloadJSON ||
		got.TraceID != in.TraceID ||
		got.CorrelationID != in.CorrelationID ||
		len(got.RequiredCapabilities) != len(in.RequiredCapabilities) {
		t.Fatalf("fields not preserved: %+v", got)
	}
}
