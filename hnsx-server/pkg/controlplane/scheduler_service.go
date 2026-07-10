package controlplane

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	iworker "github.com/hnsx-io/hnsx/server/internal/worker"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// SchedulerServiceServer implements the SchedulerService gRPC surface.
//
//   - PullSession: long-poll; server holds the call open until a session
//     matches the worker's required capabilities or the call's max_wait
//     elapses.
//   - AckSession: worker confirms it has spawned the subprocess; server
//     removes the session from the queue and stamps worker_id on the
//     session row (V1.1: in-memory only; DB integration is in #13).
//   - NackSession: worker reports a pickup failure; requeue or fail.
//   - StreamChannel: bidi. Worker streams observations up; server pushes
//     cancel / drain / ping down.
type SchedulerServiceServer struct {
	pb.UnimplementedSchedulerServiceServer
	WorkerSvc *workerservice.Service

	// DefaultMaxWaitSeconds caps the long-poll window when the worker
	// doesn't specify one. Defaults to 30.
	DefaultMaxWaitSeconds int32

	// OnObservation is called for every observation batch received from a
	// worker. The API layer uses it to fan observations out to SSE clients.
	OnObservation func(tenantID tenant.ID, sessionID string, obs *pb.Observation)

	// OnSessionStatus is called for every status update received from a
	// worker. The API layer uses it to update the in-memory session state.
	OnSessionStatus func(tenantID tenant.ID, sessionID, state string)

	mu     sync.Mutex
	active map[string]*activeSession // session_id -> bookkeeping
	// partitioned workers the chaos injector has cut off from PullSession.
	partitioned map[string]time.Time
	logf        func(format string, args ...any)
}

type activeSession struct {
	workerID  string
	tenantID  tenant.ID
	startedAt time.Time
	req       *worker.SessionRequest
}

// NewSchedulerServiceServer wires the worker service into the scheduler.
func NewSchedulerServiceServer(svc *workerservice.Service) *SchedulerServiceServer {
	return &SchedulerServiceServer{
		WorkerSvc:             svc,
		DefaultMaxWaitSeconds: 30,
		active:                map[string]*activeSession{},
		partitioned:           map[string]time.Time{},
		logf:                  log.Printf,
	}
}

// WithLogger swaps the structured log sink (used by tests).
func (s *SchedulerServiceServer) WithLogger(f func(string, ...any)) *SchedulerServiceServer {
	s.logf = f
	return s
}

// PullSession implements pb.SchedulerServiceServer.
func (s *SchedulerServiceServer) PullSession(ctx context.Context, req *pb.PullSessionRequest) (*pb.PullSessionResponse, error) {
	if s.WorkerSvc == nil {
		return nil, errors.New("scheduler: worker service not configured")
	}
	if s.isPartitioned(req.GetWorkerId()) {
		// Chaos: simulate a network partition where the worker can reach the
		// server but the server silently drops PullSession requests.
		return &pb.PullSessionResponse{}, nil
	}
	maxWait := int64(req.GetMaxWaitSeconds())
	if maxWait <= 0 {
		maxWait = int64(s.DefaultMaxWaitSeconds)
	}
	if maxWait > 60 {
		maxWait = 60 // hard cap
	}
	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(maxWait)*time.Second)
	defer cancel()

	// Derive the worker's capabilities from its registered WorkerInfo. Falls
	// back to any capabilities explicitly supplied in the request.
	required := req.GetRequiredCapabilities()
	if len(required) == 0 {
		if snap, ok := s.WorkerSvc.Get(req.GetWorkerId()); ok {
			required = iworker.CapabilitiesFromInfo(snap.Info)
		}
	}

	got, ok := s.WorkerSvc.DequeueSession(pollCtx, required)
	if !ok {
		// Empty result on timeout / cancel; the worker re-issues.
		return &pb.PullSessionResponse{}, nil
	}

	tid := tenant.DefaultID
	if snap, ok := s.WorkerSvc.Get(req.GetWorkerId()); ok && snap.Info != nil && snap.Info.GetTenantId() != "" {
		tid = tenant.ID(snap.Info.GetTenantId())
	}

	s.mu.Lock()
	s.active[got.SessionID] = &activeSession{
		workerID:  req.GetWorkerId(),
		tenantID:  tid,
		startedAt: time.Now(),
		req:       got,
	}
	s.mu.Unlock()

	s.WorkerSvc.AssignSession(req.GetWorkerId(), got.SessionID)

	return &pb.PullSessionResponse{
		SessionId:             got.SessionID,
		DomainId:              got.DomainID,
		DomainVersion:         got.DomainVersion,
		DomainSpecJson:        got.DomainSpecJSON,
		TriggerPayloadJson:    got.TriggerPayloadJSON,
		TraceId:               got.TraceID,
		AssignedAtMs:          time.Now().UnixMilli(),
		SessionTimeoutSeconds: 600,
		CorrelationId:         got.CorrelationID,
	}, nil
}

// AckSession implements pb.SchedulerServiceServer.
func (s *SchedulerServiceServer) AckSession(ctx context.Context, req *pb.AckSessionRequest) (*pb.AckSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.active[req.GetSessionId()]; !ok {
		// unknown; nothing to do (idempotent)
	}
	return &pb.AckSessionResponse{}, nil
}

// NackSession implements pb.SchedulerServiceServer.
func (s *SchedulerServiceServer) NackSession(ctx context.Context, req *pb.NackSessionRequest) (*pb.NackSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, req.GetSessionId())
	s.WorkerSvc.UnassignSession(req.GetSessionId())
	// requeue is a V1.1 placeholder; we don't yet have the original
	// session spec to put back on the queue. The client can retry by
	// re-scheduling the session via REST. V1.2 will requeue from here.
	return &pb.NackSessionResponse{}, nil
}

// StreamChannel implements the bidi stream. Server side:
//
//   - Reads worker events: observations / status / result. Logs them and
//     stores the latest status per session in “active“ so the API can
//     serve /sessions/{id} without DB.
//   - Writes server-initiated events: per-session cancel commands, drain
//     commands, periodic pings. The cancel is fed by the worker registry's
//     inbound channel; we forward those events to the stream as long as the
//     worker is sending.
//
// Recv runs in its own goroutine so the main loop can send pings and inbound
// cancel/drain events concurrently with waiting for worker messages.
func (s *SchedulerServiceServer) StreamChannel(stream pb.SchedulerService_StreamChannelServer) error {
	ctx := stream.Context()
	workerID := ""
	defer func() {
		if workerID == "" {
			return
		}
		st := s.WorkerSvc.Stats()
		if requeued := s.RequeueSessions(workerID); len(requeued) > 0 {
			s.logf("controlplane: worker=%s disconnected; requeued sessions=%v (workers=%d healthy=%d queue=%d active=%d)",
				workerID, requeued, st.Workers, st.HealthyWorkers, st.QueueLen, st.ActiveAssignments)
		}
	}()

	type recvResult struct {
		req *pb.StreamChannelRequest
		err error
	}
	recvCh := make(chan recvResult, 1)
	recvDone := make(chan struct{})
	defer close(recvDone)
	go func() {
		for {
			req, err := stream.Recv()
			select {
			case recvCh <- recvResult{req: req, err: err}:
				if err != nil {
					return
				}
			case <-recvDone:
				return
			}
		}
	}()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		// Re-resolve the inbound channel every iteration so we pick up a
		// replacement channel if the worker is evicted and re-registered.
		var workerInbound <-chan *pb.StreamChannelResponse
		if workerID != "" && s.WorkerSvc != nil {
			workerInbound = s.WorkerSvc.Registry().Inbound(workerID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pingTicker.C:
			if err := stream.Send(&pb.StreamChannelResponse{
				Payload: &pb.StreamChannelResponse_Ping{Ping: &pb.Ping{TimestampMs: time.Now().UnixMilli()}},
			}); err != nil {
				return err
			}
		case evt, ok := <-workerInbound:
			if !ok {
				continue
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		case r := <-recvCh:
			if r.err == io.EOF {
				return nil
			}
			if r.err != nil {
				return r.err
			}
			if wid := r.req.GetWorkerId(); wid != "" && workerID == "" {
				workerID = wid
				st := s.WorkerSvc.Stats()
				s.logf("controlplane: worker=%s stream connected (workers=%d healthy=%d queue=%d active=%d)",
					workerID, st.Workers, st.HealthyWorkers, st.QueueLen, st.ActiveAssignments)
			}
			s.handleWorkerEvent(r.req)
		}
	}
}

func (s *SchedulerServiceServer) handleWorkerEvent(req *pb.StreamChannelRequest) {
	switch p := req.GetPayload().(type) {
	case *pb.StreamChannelRequest_Observations:
		if p.Observations != nil {
			for _, obs := range p.Observations.GetObservations() {
				if s.OnObservation != nil {
					tid := s.tenantForSession(obs.GetSessionId())
					s.OnObservation(tid, obs.GetSessionId(), obs)
				}
			}
		}
		count := 0
		if p.Observations != nil {
			count = len(p.Observations.GetObservations())
		}
		s.logf("controlplane: worker=%s observations=%d", req.GetWorkerId(), count)
	case *pb.StreamChannelRequest_Status:
		s.mu.Lock()
		var tid tenant.ID
		if st := p.Status; st != nil {
			if act, ok := s.active[st.GetSessionId()]; ok {
				tid = act.tenantID
				// Update bookkeeping; future PRs persist to DB.
			}
			if st.GetState() == "completed" || st.GetState() == "failed" || st.GetState() == "cancelled" {
				delete(s.active, st.GetSessionId())
				s.WorkerSvc.UnassignSession(st.GetSessionId())
			}
		}
		s.mu.Unlock()
		if s.OnSessionStatus != nil && p.Status != nil {
			s.OnSessionStatus(tid, p.Status.GetSessionId(), p.Status.GetState())
		}
		s.logf("controlplane: worker=%s status session=%s state=%s", req.GetWorkerId(), p.Status.GetSessionId(), p.Status.GetState())
	case *pb.StreamChannelRequest_Result:
		s.logf("controlplane: worker=%s result session=%s", req.GetWorkerId(), p.Result.GetSessionId())
	default:
		s.logf("controlplane: worker=%s unknown event", req.GetWorkerId())
	}
}

func (s *SchedulerServiceServer) tenantForSession(sessionID string) tenant.ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if act, ok := s.active[sessionID]; ok {
		return act.tenantID
	}
	return tenant.DefaultID
}

// ActiveSessions returns a copy of the in-memory session bookkeeping for
// diagnostics. Not part of the gRPC surface.
func (s *SchedulerServiceServer) ActiveSessions() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.active))
	for k, v := range s.active {
		out[k] = v.workerID
	}
	return out
}

// RequeueSessions puts all in-flight sessions assigned to workerID back onto
// the scheduling queue. This is called when a worker is evicted or its
// StreamChannel ends unexpectedly. It returns the requeued session IDs.
func (s *SchedulerServiceServer) RequeueSessions(workerID string) []string {
	if s.WorkerSvc == nil || workerID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var requeued []string
	for sid, act := range s.active {
		if act.workerID != workerID {
			continue
		}
		if act.req != nil {
			s.WorkerSvc.EnqueueSession(act.req)
			requeued = append(requeued, sid)
		}
		delete(s.active, sid)
	}
	return requeued
}

// PartitionPull simulates a network partition for workerID: for the next d
// the server will silently return empty PullSession responses. Used by the
// chaos test suite to verify that sessions are not lost when a worker is
// temporarily isolated but later recovers.
func (s *SchedulerServiceServer) PartitionPull(workerID string, d time.Duration) {
	if workerID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.partitioned == nil {
		s.partitioned = map[string]time.Time{}
	}
	s.partitioned[workerID] = time.Now().Add(d)
}

func (s *SchedulerServiceServer) isPartitioned(workerID string) bool {
	if workerID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.partitioned == nil {
		return false
	}
	until, ok := s.partitioned[workerID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(s.partitioned, workerID)
		return false
	}
	return true
}
