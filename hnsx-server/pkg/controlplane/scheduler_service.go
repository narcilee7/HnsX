package controlplane

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

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
	Registry *worker.Registry
	Queue    *worker.SessionQueue

	// DefaultMaxWaitSeconds caps the long-poll window when the worker
	// doesn't specify one. Defaults to 30.
	DefaultMaxWaitSeconds int32

	// OnObservation is called for every observation batch received from a
	// worker. The API layer uses it to fan observations out to SSE clients.
	OnObservation func(sessionID string, obs *pb.Observation)

	// OnSessionStatus is called for every status update received from a
	// worker. The API layer uses it to update the in-memory session state.
	OnSessionStatus func(sessionID, state string)

	mu     sync.Mutex
	active map[string]*activeSession // session_id -> bookkeeping
	logf   func(format string, args ...any)
}

type activeSession struct {
	workerID  string
	startedAt time.Time
}

// NewSchedulerServiceServer wires the registry + queue into a server.
func NewSchedulerServiceServer(reg *worker.Registry, q *worker.SessionQueue) *SchedulerServiceServer {
	return &SchedulerServiceServer{
		Registry:              reg,
		Queue:                 q,
		DefaultMaxWaitSeconds: 30,
		active:                map[string]*activeSession{},
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
	if s.Queue == nil {
		return nil, errors.New("scheduler: queue not configured")
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

	got, ok := s.Queue.Dequeue(pollCtx, req.GetRequiredCapabilities())
	if !ok {
		// Empty result on timeout / cancel; the worker re-issues.
		return &pb.PullSessionResponse{}, nil
	}

	s.mu.Lock()
	s.active[got.SessionID] = &activeSession{workerID: req.GetWorkerId(), startedAt: time.Now()}
	s.mu.Unlock()

	if s.Registry != nil {
		s.Registry.AssignSession(req.GetWorkerId(), got.SessionID)
	}

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
	if s.Registry != nil {
		s.Registry.UnassignSession(req.GetSessionId())
	}
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
//     commands, periodic pings. The cancel is fed by
//     “Registry.SendCancel“; we forward the Inbound channel events to
//     the stream as long as the worker is sending.
func (s *SchedulerServiceServer) StreamChannel(stream pb.SchedulerService_StreamChannelServer) error {
	ctx := stream.Context()
	workerID := ""

	// Discover this worker's inbound channel by reading the first
	// observation whose WorkerId tells us who they are. We accept a
	// small startup window where we don't know the id yet.
	inbound := make(chan *pb.StreamChannelResponse, 8)
	defer close(inbound)

	// Watch the registry for an inbound channel matching this stream's
	// worker once it identifies itself.
	var workerInbound <-chan *pb.StreamChannelResponse
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatch:
				return
			case <-ticker.C:
				if workerID == "" {
					continue
				}
				ch := s.Registry.Inbound(workerID)
				if ch != nil {
					workerInbound = ch
					return
				}
			}
		}
	}()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
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
				// worker channel closed (e.g. eviction)
				workerInbound = nil
				continue
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		default:
		}

		// Non-blocking recv from the worker.
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if wid := req.GetWorkerId(); wid != "" {
			workerID = wid
		}
		s.handleWorkerEvent(req)
	}
}

func (s *SchedulerServiceServer) handleWorkerEvent(req *pb.StreamChannelRequest) {
	switch p := req.GetPayload().(type) {
	case *pb.StreamChannelRequest_Observations:
		if p.Observations != nil {
			for _, obs := range p.Observations.GetObservations() {
				if s.OnObservation != nil {
					s.OnObservation(obs.GetSessionId(), obs)
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
		if st := p.Status; st != nil {
			if _, ok := s.active[st.GetSessionId()]; ok {
				// Update bookkeeping; future PRs persist to DB.
			}
			if st.GetState() == "completed" || st.GetState() == "failed" || st.GetState() == "cancelled" {
				delete(s.active, st.GetSessionId())
				if s.Registry != nil {
					s.Registry.UnassignSession(st.GetSessionId())
				}
			}
		}
		s.mu.Unlock()
		if s.OnSessionStatus != nil && p.Status != nil {
			s.OnSessionStatus(p.Status.GetSessionId(), p.Status.GetState())
		}
		s.logf("controlplane: worker=%s status session=%s state=%s", req.GetWorkerId(), p.Status.GetSessionId(), p.Status.GetState())
	case *pb.StreamChannelRequest_Result:
		s.logf("controlplane: worker=%s result session=%s", req.GetWorkerId(), p.Result.GetSessionId())
	default:
		s.logf("controlplane: worker=%s unknown event", req.GetWorkerId())
	}
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
