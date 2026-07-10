package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/worker"
	workerrepository "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func newTestServer(t *testing.T) (*SchedulerServiceServer, *worker.Registry, worker.SessionQueue) {
	t.Helper()
	reg := worker.NewRegistry()
	reg.SetClock(func() time.Time { return time.Unix(0, 0) })
	q := worker.NewSessionQueue()
	svc := workerservice.NewServiceWithQueue(workerrepository.NewInMemoryRepository(), q).WithRegistry(reg)
	s := NewSchedulerServiceServer(svc)
	return s, reg, q
}

func newTestWorkerService(t *testing.T) *workerservice.Service {
	t.Helper()
	return workerservice.NewServiceWithQueue(workerrepository.NewInMemoryRepository(), worker.NewSessionQueue()).WithRegistry(worker.NewRegistry())
}

func TestScheduler_PullSession_returnsEmptyWhenNoWork(t *testing.T) {
	s, _, _ := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	resp, err := s.PullSession(ctx, &pb.PullSessionRequest{MaxWaitSeconds: 1})
	if err != nil {
		t.Fatalf("PullSession: %v", err)
	}
	if resp.GetSessionId() != "" {
		t.Fatalf("expected empty session_id, got %q", resp.GetSessionId())
	}
}

func TestScheduler_PullSession_returnsEnqueuedSession(t *testing.T) {
	s, _, q := newTestServer(t)
	q.Enqueue(&worker.SessionRequest{
		SessionID:          "s-1",
		DomainID:           "d-1",
		DomainSpecJSON:     `{"id":"d-1"}`,
		TriggerPayloadJSON: `{"q":"hi"}`,
	})

	resp, err := s.PullSession(context.Background(), &pb.PullSessionRequest{WorkerId: "w-1", MaxWaitSeconds: 5})
	if err != nil {
		t.Fatalf("PullSession: %v", err)
	}
	if resp.GetSessionId() != "s-1" {
		t.Fatalf("expected s-1, got %q", resp.GetSessionId())
	}
	if resp.GetDomainId() != "d-1" {
		t.Fatalf("expected d-1, got %q", resp.GetDomainId())
	}
	if got := s.ActiveSessions(); got["s-1"] != "w-1" {
		t.Fatalf("active session bookkeeping = %v", got)
	}
}

func TestScheduler_PullSession_respectsRequiredCapabilities(t *testing.T) {
	s, _, q := newTestServer(t)
	q.Enqueue(&worker.SessionRequest{
		SessionID:            "s-anthropic",
		DomainID:             "d",
		RequiredCapabilities: []string{"provider:anthropic"},
	})
	q.Enqueue(&worker.SessionRequest{
		SessionID:            "s-openai",
		DomainID:             "d",
		RequiredCapabilities: []string{"provider:openai"},
	})

	resp, err := s.PullSession(context.Background(), &pb.PullSessionRequest{
		WorkerId:             "w-openai",
		MaxWaitSeconds:       1,
		RequiredCapabilities: []string{"provider:openai"},
	})
	if err != nil {
		t.Fatalf("PullSession: %v", err)
	}
	if resp.GetSessionId() != "s-openai" {
		t.Fatalf("expected s-openai, got %q", resp.GetSessionId())
	}
}

func TestWorkerService_Register_thenHeartbeat(t *testing.T) {
	svc := newTestWorkerService(t)
	srv := NewWorkerServiceServer(svc)

	resp, err := srv.Register(context.Background(), &pb.RegisterRequest{
		Info: &pb.WorkerInfo{WorkerId: "w-1", Region: "local"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.GetWorkerId() != "w-1" {
		t.Fatalf("worker_id = %q, want w-1", resp.GetWorkerId())
	}
	if resp.GetHeartbeatIntervalSeconds() != 5 {
		t.Fatalf("default heartbeat = %d, want 5", resp.GetHeartbeatIntervalSeconds())
	}

	srv.HeartbeatIntervalSeconds = 2
	resp, _ = srv.Register(context.Background(), &pb.RegisterRequest{
		Info: &pb.WorkerInfo{Region: "local"},
	})
	if resp.GetHeartbeatIntervalSeconds() != 2 {
		t.Fatalf("custom heartbeat = %d, want 2", resp.GetHeartbeatIntervalSeconds())
	}

	if _, err := srv.Heartbeat(context.Background(), &pb.HeartbeatRequest{WorkerId: "w-1"}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if _, err := srv.Heartbeat(context.Background(), &pb.HeartbeatRequest{WorkerId: "w-unknown"}); err == nil {
		t.Fatalf("expected error for unknown worker")
	}
}

func TestScheduler_AckSession_removesFromActive(t *testing.T) {
	s, _, q := newTestServer(t)
	q.Enqueue(&worker.SessionRequest{SessionID: "s-1", DomainID: "d"})
	_, _ = s.PullSession(context.Background(), &pb.PullSessionRequest{WorkerId: "w-1", MaxWaitSeconds: 1})

	if _, ok := s.ActiveSessions()["s-1"]; !ok {
		t.Fatalf("expected s-1 in active sessions before ack")
	}
	// Manually clear via NackSession (AckSession is a no-op for
	// bookkeeping in V1.1).
	if _, err := s.NackSession(context.Background(), &pb.NackSessionRequest{WorkerId: "w-1", SessionId: "s-1"}); err != nil {
		t.Fatalf("NackSession: %v", err)
	}
	if _, ok := s.ActiveSessions()["s-1"]; ok {
		t.Fatalf("expected s-1 removed from active after nack")
	}
}

func TestScheduler_RequeueSessions_onDisconnect(t *testing.T) {
	s, _, q := newTestServer(t)
	q.Enqueue(&worker.SessionRequest{SessionID: "s-1", DomainID: "d"})
	q.Enqueue(&worker.SessionRequest{SessionID: "s-2", DomainID: "d"})

	_, _ = s.PullSession(context.Background(), &pb.PullSessionRequest{WorkerId: "w-1", MaxWaitSeconds: 1})
	_, _ = s.PullSession(context.Background(), &pb.PullSessionRequest{WorkerId: "w-2", MaxWaitSeconds: 1})

	if q.Len() != 0 {
		t.Fatalf("queue should be empty after pull, got %d", q.Len())
	}

	requeued := s.RequeueSessions("w-1")
	if len(requeued) != 1 || requeued[0] != "s-1" {
		t.Fatalf("requeued = %v, want [s-1]", requeued)
	}

	if q.Len() != 1 {
		t.Fatalf("queue should have 1 requeued session, got %d", q.Len())
	}

	// Active bookkeeping for w-1 should be gone.
	if _, ok := s.ActiveSessions()["s-1"]; ok {
		t.Fatal("expected s-1 removed from active after requeue")
	}
	// w-2 session should remain active.
	if _, ok := s.ActiveSessions()["s-2"]; !ok {
		t.Fatal("expected s-2 to remain active")
	}
}
