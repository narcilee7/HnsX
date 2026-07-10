package controlplane

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/worker"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// TestChaos_SessionsSurviveWorkerEvictionAndPartition verifies that no
// session is lost when workers are randomly evicted or network-partitioned
// while sessions are still pending or in-flight.
func TestChaos_SessionsSurviveWorkerEvictionAndPartition(t *testing.T) {
	reg := worker.NewRegistry()
	q := worker.NewMemorySessionQueue()
	sched := NewSchedulerServiceServer(reg, q)
	chaos := &ChaosInjector{Registry: reg, Sched: sched}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const (
		numWorkers  = 3
		numSessions = 20
	)

	// Register workers.
	for i := 0; i < numWorkers; i++ {
		_, _ = reg.Register(&pb.WorkerInfo{WorkerId: fmt.Sprintf("w-%d", i)})
	}

	pulled := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup
	workerCtx, workerCancel := context.WithCancel(ctx)

	// Start fake workers that continuously pull sessions.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		wid := fmt.Sprintf("w-%d", i)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-workerCtx.Done():
					return
				default:
				}
				resp, err := sched.PullSession(workerCtx, &pb.PullSessionRequest{
					WorkerId:       wid,
					MaxWaitSeconds: 1,
				})
				if err != nil {
					return
				}
				if resp.GetSessionId() == "" {
					continue
				}
				mu.Lock()
				pulled[resp.GetSessionId()]++
				mu.Unlock()
			}
		}()
	}

	// Enqueue sessions.
	for i := 0; i < numSessions; i++ {
		q.Enqueue(&worker.SessionRequest{SessionID: fmt.Sprintf("s-%d", i), DomainID: "d"})
	}

	// Chaos loop: randomly evict or partition workers while sessions remain.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if q.Len() == 0 {
					return
				}
				switch rand.Intn(3) {
				case 0:
					wid := fmt.Sprintf("w-%d", rand.Intn(numWorkers))
					chaos.EvictWorker(wid)
					// Re-register so the worker pool can recover.
					_, _ = reg.Register(&pb.WorkerInfo{WorkerId: wid})
				case 1:
					wid := fmt.Sprintf("w-%d", rand.Intn(numWorkers))
					chaos.PartitionPull(wid, 200*time.Millisecond)
				}
			}
		}
	}()

	// Wait until the queue is drained.
	for {
		if ctx.Err() != nil {
			break
		}
		if q.Len() == 0 {
			// Give workers a moment to pull the last requeued items.
			time.Sleep(200 * time.Millisecond)
			if q.Len() == 0 {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	workerCancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(pulled) != numSessions {
		t.Fatalf("pulled %d distinct sessions, want %d", len(pulled), numSessions)
	}
	for i := 0; i < numSessions; i++ {
		sid := fmt.Sprintf("s-%d", i)
		if pulled[sid] == 0 {
			t.Fatalf("session %s was never pulled", sid)
		}
	}
	t.Logf("chaos test complete: all %d sessions pulled (total pulls=%d)", numSessions, func() int {
		n := 0
		for _, c := range pulled {
			n += c
		}
		return n
	}())
}
