package controlplane

import (
	"context"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/worker"
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
	Registry *worker.Registry
	// HeartbeatIntervalSeconds is the cadence the server asks workers
	// to honor. Defaults to 5; tests can shorten it.
	HeartbeatIntervalSeconds int32
}

// Register implements pb.WorkerServiceServer.
func (s *WorkerServiceServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if s.Registry == nil {
		return nil, errNoRegistry
	}
	wid, err := s.Registry.Register(req.GetInfo())
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
	if s.Registry == nil {
		return nil, errNoRegistry
	}
	if err := s.Registry.Heartbeat(req.GetWorkerId(), req); err != nil {
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

var errNoRegistry = errControlplane("controlplane: registry not configured")

type errControlplane string

func (e errControlplane) Error() string { return string(e) }
