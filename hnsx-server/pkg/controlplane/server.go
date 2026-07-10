// Package controlplane is the gRPC control plane entrypoint.
//
// V1.1 (Python Worker Pivot) registers the two new services from
// proto/hnsx/v1/worker.proto: WorkerService and SchedulerService.
// They share a worker.Registry and worker.SessionQueue owned by the
// server's caller (see cmd/hnsx-server). The legacy services
// (DomainRegistryService, etc.) remain available for the Go-side API
// and console.
package controlplane

import (
	"context"
	"net"
	"sync"

	"google.golang.org/grpc"

	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// Server is the gRPC control-plane server.
type Server struct {
	addr     string
	mu       sync.Mutex
	listener net.Listener
	gs       *grpc.Server

	// Worker / scheduler services; nil-safe (the gRPC server still
	// starts even if these aren't wired, but the corresponding RPCs
	// will return Unimplemented).
	Worker *WorkerServiceServer
	Sched  *SchedulerServiceServer
}

// NewServer constructs a Server bound to addr.
func NewServer(addr string) *Server { return &Server{addr: addr} }

// WithWorkerService wires the V1.1 worker service into the gRPC server.
// The service owns the registry and queue shared with the API layer so REST
// session creation can enqueue and REST cancel can publish to the worker's
// StreamChannel.
func (s *Server) WithWorkerService(svc *workerservice.Service) *Server {
	s.Worker = NewWorkerServiceServer(svc)
	s.Sched = NewSchedulerServiceServer(svc)
	return s
}

// Addr returns the listen address (only meaningful after ListenAndServe).
func (s *Server) Addr() string { return s.addr }

// ListenAndServe opens the TCP listener and blocks until ctx is canceled
// or the underlying gRPC server fails. Callers should use Shutdown for a
// graceful stop rather than relying on ctx cancellation alone.
func (s *Server) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = l
	s.gs = grpc.NewServer(
		grpc.UnaryInterceptor(tenant.UnaryServerInterceptor),
		grpc.StreamInterceptor(tenant.StreamServerInterceptor),
	)
	if s.Worker != nil {
		pb.RegisterWorkerServiceServer(s.gs, s.Worker)
	}
	if s.Sched != nil {
		pb.RegisterSchedulerServiceServer(s.gs, s.Sched)
	}
	s.mu.Unlock()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.gs.Serve(l)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-serveErr:
		return err
	}
}

// Shutdown gracefully stops the gRPC server. If the graceful stop does not
// finish within the context deadline, it falls back to an immediate stop.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	gs := s.gs
	s.mu.Unlock()
	if gs == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		gs.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		gs.Stop()
		return ctx.Err()
	}
}
