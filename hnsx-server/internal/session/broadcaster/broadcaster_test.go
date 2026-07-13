package broadcaster

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func TestBroadcaster_PublishSubscribe(t *testing.T) {
	b := NewBroadcaster()
	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	obs := domain.Observation{Kind: "session_start", SessionID: "s1"}
	if err := b.Publish(context.Background(), obs); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case got := <-ch:
		if got.Kind != "session_start" {
			t.Fatalf("got kind %q", got.Kind)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestBroadcaster_ReplayBuffer_LateSubscribers(t *testing.T) {
	b := NewBroadcaster()
	ctx := context.Background()

	// Two events go in BEFORE anyone subscribes.
	_ = b.Publish(ctx, domain.Observation{Kind: "session_start", SessionID: "s1"})
	_ = b.Publish(ctx, domain.Observation{Kind: "step_start", SessionID: "s1", AgentID: "triage"})

	// Now subscribe.
	ch, unsub := b.Subscribe()
	defer unsub()

	got1 := <-ch
	if got1.Kind != "session_start" {
		t.Fatalf("replay[0] = %q", got1.Kind)
	}
	got2 := <-ch
	if got2.Kind != "step_start" {
		t.Fatalf("replay[1] = %q", got2.Kind)
	}
}

func TestBroadcaster_MultipleSubscribers_FanOut(t *testing.T) {
	b := NewBroadcaster()
	ch1, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	const N = 5
	for i := 0; i < N; i++ {
		_ = b.Publish(context.Background(), domain.Observation{Kind: "tick"})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	collect := func(ch <-chan domain.Observation, name string) {
		defer wg.Done()
		count := 0
		for e := range ch {
			if e.Kind == "tick" {
				count++
			}
			if count == N {
				return
			}
		}
		if count != N {
			t.Errorf("[%s] got %d, want %d", name, count, N)
		}
	}
	go collect(ch1, "ch1")
	go collect(ch2, "ch2")
	wg.Wait()
}

func TestBroadcaster_CloseUnsubscribes(t *testing.T) {
	b := NewBroadcaster()
	ch, unsub := b.Subscribe()
	b.Close()

	// Channel should be closed.
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("expected closed channel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}
	// Idempotent.
	b.Close()
	// Unsubscribe after close is safe.
	unsub()
}

func TestBroadcaster_PublishAfterClose_Noop(t *testing.T) {
	b := NewBroadcaster()
	b.Close()
	if err := b.Publish(context.Background(), domain.Observation{Kind: "after_close"}); err != nil {
		t.Fatalf("publish after close: %v", err)
	}
}
