package worker

import (
	"errors"
	"testing"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func newTestRegistry() *Registry {
	r := NewRegistry()
	// Pin the clock to make tests deterministic.
	r.SetClock(func() time.Time { return time.Unix(0, 0) })
	return r
}

func TestRegistry_Register_mintsIDWhenEmpty(t *testing.T) {
	r := newTestRegistry()
	wid, err := r.Register(&pb.WorkerInfo{Region: "us-west-2"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if wid == "" {
		t.Fatalf("Register returned empty worker_id")
	}
	if got, ok := r.Get(wid); !ok || got.Info.GetRegion() != "us-west-2" {
		t.Fatalf("Get(%q) = %v, %v", wid, got, ok)
	}
}

func TestRegistry_Register_preservesSuppliedID(t *testing.T) {
	r := newTestRegistry()
	wid, err := r.Register(&pb.WorkerInfo{WorkerId: "w-fixed", Region: "local"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if wid != "w-fixed" {
		t.Fatalf("expected w-fixed, got %q", wid)
	}
}

func TestRegistry_Register_isIdempotent(t *testing.T) {
	r := newTestRegistry()
	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-1", Region: "local"})
	got, _ := r.Get("w-1")
	originalInbound := r.Inbound("w-1")
	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-1", Region: "us-west-2"})
	got2, _ := r.Get("w-1")
	if got.Info.GetRegion() != "local" || got2.Info.GetRegion() != "us-west-2" {
		t.Fatalf("info not updated: %v -> %v", got.Info, got2.Info)
	}
	if r.Inbound("w-1") != originalInbound {
		t.Fatalf("re-Register must preserve the Inbound channel")
	}
}

func TestRegistry_Heartbeat_UnknownReturnsError(t *testing.T) {
	r := newTestRegistry()
	err := r.Heartbeat("w-nope", &pb.HeartbeatRequest{})
	if !errors.Is(err, ErrUnknownWorker) {
		t.Fatalf("expected ErrUnknownWorker, got %v", err)
	}
}

func TestRegistry_Heartbeat_UpdatesLastSeen(t *testing.T) {
	r := newTestRegistry()
	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-1"})
	r.SetClock(func() time.Time { return time.Unix(10, 0) })
	if err := r.Heartbeat("w-1", &pb.HeartbeatRequest{Usage: &pb.ResourceUsage{RunningSessions: 1}}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	snap, _ := r.Get("w-1")
	if snap.LastSeen.Unix() != 10 {
		t.Fatalf("LastSeen = %v, want 10", snap.LastSeen)
	}
	if !snap.Healthy {
		t.Fatalf("expected Healthy=true (age=0)")
	}
}

func TestRegistry_EvictStale_RemovesAndCloses(t *testing.T) {
	r := newTestRegistry()
	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-old"})
	r.SetClock(func() time.Time { return time.Unix(60, 0) })
	evicted := r.EvictStale(30 * time.Second)
	if len(evicted) != 1 || evicted[0] != "w-old" {
		t.Fatalf("evicted = %v, want [w-old]", evicted)
	}
	if r.Len() != 0 {
		t.Fatalf("Len = %d, want 0", r.Len())
	}
	// After eviction, the worker is gone — Inbound returns nil.
	if got := r.Inbound("w-old"); got != nil {
		t.Fatalf("Inbound after eviction = %v, want nil", got)
	}
	// SendCancel becomes a no-op too.
	if r.SendCancel("w-old", "s-1", "test", 0) {
		t.Fatalf("SendCancel after eviction should return false")
	}
}

func TestRegistry_SendCancel_DropsWhenFull(t *testing.T) {
	r := newTestRegistry()
	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-1"})
	ch := r.Inbound("w-1")
	for i := 0; i < 32; i++ {
		if !r.SendCancel("w-1", "s-1", "test", 0) {
			t.Fatalf("SendCancel at i=%d unexpectedly dropped", i)
		}
	}
	if r.SendCancel("w-1", "s-1", "test", 0) {
		t.Fatalf("expected drop when channel is full")
	}
	<-ch
	if !r.SendCancel("w-1", "s-1", "test", 0) {
		t.Fatalf("expected SendCancel to succeed after drain")
	}
}

func TestRegistry_SendCancel_UnknownWorker(t *testing.T) {
	r := newTestRegistry()
	if r.SendCancel("w-nope", "s-1", "x", 0) {
		t.Fatalf("expected SendCancel to return false for unknown worker")
	}
}
