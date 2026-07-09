// Package controlplane is the gRPC control plane entrypoint. Phase 1 keeps
// the gRPC interface as a stub — the active API surface is REST + SSE in
// pkg/api, which the console consumes.
//
// Future PRs will register the v1 services from proto/hnsx/v1/control_plane.proto
// (DomainRegistryService, SessionSchedulerService, TelemetryService, etc.).
package controlplane

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// Server is the gRPC control-plane server.
type Server struct {
	addr     string
	mu       sync.Mutex
	listener net.Listener
	gs       *grpc.Server
}

// NewServer constructs a Server bound to addr.
func NewServer(addr string) *Server { return &Server{addr: addr} }

// Addr returns the listen address.
func (s *Server) Addr() string { return s.addr }

// ListenAndServe opens the TCP listener and blocks until ctx is canceled or
// the underlying gRPC server fails.
func (s *Server) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = l
	s.gs = grpc.NewServer()
	s.mu.Unlock()

	// Future: register pb.RegisterDomainRegistryServiceServer(s.gs, ...)
	// while the registry list grows.
	_ = pb.File_hnsx_v1_domain_proto

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.gs.Serve(l)
	}()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		gs := s.gs
		s.mu.Unlock()
		if gs != nil {
			done := make(chan struct{})
			go func() {
				gs.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				gs.Stop()
			}
		}
		return ctx.Err()
	case err := <-serveErr:
		return err
	}
}
