package worker

import (
	"testing"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func TestRegistry_AssignSession_tracksPerWorker(t *testing.T) {
	r := NewRegistry()
	r.AssignSession("w-1", "s-1")
	r.AssignSession("w-1", "s-2")
	r.AssignSession("w-2", "s-3")

	w1 := r.SessionsForWorker("w-1")
	if len(w1) != 2 {
		t.Fatalf("w-1 sessions = %d, want 2", len(w1))
	}

	w2 := r.SessionsForWorker("w-2")
	if len(w2) != 1 {
		t.Fatalf("w-2 sessions = %d, want 1", len(w2))
	}
}

func TestRegistry_UnassignSession_cleansWorkerSet(t *testing.T) {
	r := NewRegistry()
	r.AssignSession("w-1", "s-1")
	r.UnassignSession("s-1")

	if len(r.SessionsForWorker("w-1")) != 0 {
		t.Fatalf("expected w-1 sessions to be empty")
	}
	if _, ok := r.SessionWorker("s-1"); ok {
		t.Fatal("expected s-1 to have no worker")
	}
}

func TestRegistry_AssignSession_reassign(t *testing.T) {
	r := NewRegistry()
	r.AssignSession("w-1", "s-1")
	r.AssignSession("w-2", "s-1")

	if len(r.SessionsForWorker("w-1")) != 0 {
		t.Fatalf("expected s-1 removed from w-1")
	}
	if len(r.SessionsForWorker("w-2")) != 1 {
		t.Fatalf("expected s-1 assigned to w-2")
	}
	wid, _ := r.SessionWorker("s-1")
	if wid != "w-2" {
		t.Fatalf("session worker = %q, want w-2", wid)
	}
}

func TestRegistry_EvictStale_removesWorkerAndSessions(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	r.SetClock(func() time.Time { return now })

	_, _ = r.Register(&pb.WorkerInfo{WorkerId: "w-old"})
	r.AssignSession("w-old", "s-1")

	// Advance time past eviction threshold.
	r.SetClock(func() time.Time { return now.Add(2 * time.Minute) })

	evicted := r.EvictStale(60 * time.Second)
	if len(evicted) != 1 || evicted[0] != "w-old" {
		t.Fatalf("evicted = %v, want [w-old]", evicted)
	}
	if _, ok := r.Get("w-old"); ok {
		t.Fatal("expected w-old to be evicted")
	}
	if len(r.SessionsForWorker("w-old")) != 0 {
		t.Fatal("expected w-old sessions to be cleared")
	}
}

func TestRegistry_RemoveWorkerSessions(t *testing.T) {
	r := NewRegistry()
	r.AssignSession("w-1", "s-1")
	r.AssignSession("w-1", "s-2")

	r.RemoveWorkerSessions("w-1")

	if len(r.SessionsForWorker("w-1")) != 0 {
		t.Fatal("expected w-1 sessions to be removed")
	}
	if _, ok := r.SessionWorker("s-1"); ok {
		t.Fatal("expected s-1 assignment removed")
	}
	if _, ok := r.SessionWorker("s-2"); ok {
		t.Fatal("expected s-2 assignment removed")
	}
}
