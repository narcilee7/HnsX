package controlplane

import (
	"context"
	"time"

	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// WorkerServiceServer implements the WorkerService gRPC surface that
// Python workers call into. The two RPCs are:
//
//   - Register  : first call after a worker process starts; returns the
//     canonical worker_id and the server's preferred
//     heartbeat cadence.
//   - Heartbeat : every ~5s; carries resource usage + liveness signal.
type WorkerServiceServer struct {
	pb.UnimplementedWorkerServiceServer
	WorkerSvc *workerservice.Service
	// HeartbeatIntervalSeconds is the cadence the server asks workers
	// to honor. Defaults to 5; tests can shorten it.
	HeartbeatIntervalSeconds int32
}

// NewWorkerServiceServer constructs a WorkerServiceServer backed by the
// supplied worker service.
func NewWorkerServiceServer(svc *workerservice.Service) *WorkerServiceServer {
	return &WorkerServiceServer{WorkerSvc: svc}
}

// Register implements pb.WorkerServiceServer.
func (s *WorkerServiceServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if s.WorkerSvc == nil {
		return nil, errNoWorkerService
	}
	wid, err := s.WorkerSvc.Register(req.GetInfo())
	if err != nil {
		return nil, err
	}
	return &pb.RegisterResponse{
		WorkerId:                 wid,
		ServerTimeMs:             time.Now().UnixMilli(),
		HeartbeatIntervalSeconds: s.heartbeatInterval(),
	}, nil
}

// Heartbeat implements pb.WorkerServiceServer.
func (s *WorkerServiceServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if s.WorkerSvc == nil {
		return nil, errNoWorkerService
	}
	if err := s.WorkerSvc.Heartbeat(req.GetWorkerId(), req); err != nil {
		return nil, err
	}
	return &pb.HeartbeatResponse{
		ServerTimeMs: time.Now().UnixMilli(),
	}, nil
}

func (s *WorkerServiceServer) heartbeatInterval() int32 {
	if s.HeartbeatIntervalSeconds > 0 {
		return s.HeartbeatIntervalSeconds
	}
	return 5
}

var errNoWorkerService = errControlplane("controlplane: worker service not configured")

type errControlplane string

func (e errControlplane) Error() string { return string(e) }
