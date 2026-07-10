// Package bootstrap contains the shared startup/shutdown logic for the
// hnsx-server control plane. It keeps cmd/hnsx-server/main.go as a thin
// wrapper and makes the server lifecycle testable outside of main.
package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	stdruntime "runtime"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/config"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/api"
	"github.com/hnsx-io/hnsx/server/pkg/controlplane"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/version"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// Server wires the full hnsx-server control plane.
type Server struct {
	Config      *config.Config
	Application *app.Application
	APIServer   *api.Server
	GRPCServer  *controlplane.Server
}

// NewServerFromArgs builds a Server from the "server" subcommand arguments.
func NewServerFromArgs(args []string) (*Server, error) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "optional path to YAML config")
	seedFrom := fs.String("seed-from", "", "optional directory of v2 DomainSpec YAMLs to register on boot (development only)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()
	application, err := app.NewApplication(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("application: %w", err)
	}

	build := api.BuildInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		Built:     version.Built,
		GoVersion: stdruntime.Version(),
	}
	apiServer := api.NewServerWithWorkerPool(build, application)

	if *seedFrom != "" {
		seedFromDir(apiServer, *seedFrom)
	}

	s := &Server{
		Config:      cfg,
		Application: application,
		APIServer:   apiServer,
	}

	if cfg.GRPCAddr != "" {
		grpcSrv := controlplane.NewServer(cfg.GRPCAddr).WithWorkerService(application.WorkerService)
		if grpcSrv.Sched != nil {
			grpcSrv.Sched.OnObservation = func(tid tenant.ID, sessionID string, obs *pb.Observation) {
				payload := map[string]any{}
				if obs.GetPayload() != "" {
					_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
				}
				apiServer.PublishObservation(sessionID, runtime.Observation{
					Kind:      obs.GetKind(),
					SessionID: obs.GetSessionId(),
					DomainID:  obs.GetDomainId(),
					StepID:    obs.GetStepId(),
					AgentID:   obs.GetAgentId(),
					ParentID:  obs.GetParentId(),
					TraceID:   obs.GetTraceId(),
					Payload:   payload,
					Timestamp: time.UnixMilli(obs.GetCreatedAtMs()),
				})
			}
			grpcSrv.Sched.OnSessionStatus = func(tid tenant.ID, sessionID, state string) {
				if application.SessionService != nil {
					_, _ = application.SessionService.UpdateState(sessionID, sessionmodel.State(state))
				}
			}
		}
		s.GRPCServer = grpcSrv
	}

	return s, nil
}

// Run starts the HTTP server and optional gRPC server and blocks until ctx is
// canceled or a server fails. It performs a graceful shutdown before returning.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.APIServer.Listen(s.Config.HTTPAddr)
	}()

	var grpcErr chan error
	if s.GRPCServer != nil {
		grpcErr = make(chan error, 1)
		go func() {
			grpcErr <- s.GRPCServer.ListenAndServe(ctx)
		}()
	}

	var stopGC chan struct{}
	if s.Application.WorkerService != nil {
		stopGC = make(chan struct{})
		go s.runStaleWorkerGC(stopGC)
	}

	log.Printf("[hnsx-server] listening on http=%s grpc=%s", s.Config.HTTPAddr, s.Config.GRPCAddr)
	log.Printf("[hnsx-server] version=%s commit=%s", s.APIServer.BuildInfo.Version, s.APIServer.BuildInfo.Commit)
	log.Printf("[hnsx-server] otel=%s build=%s", s.Config.OTel.Exporter, s.APIServer.BuildInfo.GoVersion)

	select {
	case <-ctx.Done():
		log.Println("[hnsx-server] shutting down")
		if stopGC != nil {
			close(stopGC)
		}
		return s.shutdown()
	case err := <-serveErr:
		return fmt.Errorf("http: %w", err)
	case err := <-grpcErr:
		return fmt.Errorf("grpc: %w", err)
	}
}

// shutdown stops HTTP, gRPC, and the application gracefully.
func (s *Server) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.APIServer.Drain(shutdownCtx); err != nil {
		log.Printf("[hnsx-server] api drain: %v", err)
	}
	if err := s.APIServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[hnsx-server] api shutdown: %v", err)
	}
	if s.GRPCServer != nil {
		if err := s.GRPCServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] grpc shutdown: %v", err)
		}
	}
	if err := s.Application.Close(shutdownCtx); err != nil {
		log.Printf("[hnsx-server] application close: %v", err)
	}
	return nil
}

func (s *Server) runStaleWorkerGC(stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if s.Application.WorkerService == nil {
				continue
			}
			evicted := s.Application.WorkerService.EvictStale(60 * time.Second)
			if len(evicted) == 0 {
				continue
			}
			log.Printf("[hnsx-server] evicted %d stale worker(s): %v", len(evicted), evicted)
			if s.GRPCServer == nil || s.GRPCServer.Sched == nil {
				continue
			}
			for _, wid := range evicted {
				if requeued := s.GRPCServer.Sched.RequeueSessions(wid); len(requeued) > 0 {
					log.Printf("[hnsx-server] requeued %d session(s) from worker %s", len(requeued), wid)
				}
			}
		}
	}
}

// seedFromDir walks the given directory and registers every v2 DomainSpec YAML
// against the API server. It is an explicit operator action (--seed-from) so
// production deployments never implicitly pull in development fixtures.
func seedFromDir(s *api.Server, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[seed] cannot read %s: %v (skipping)", dir, err)
		return
	}
	registered := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := fmt.Sprintf("%s/%s/domain.yaml", dir, e.Name())
		spec, err := spec.LoadFile(path)
		if err != nil {
			log.Printf("[seed] skip %s: %v", path, err)
			continue
		}
		s.RegisterBootstrapDomain(tenant.DefaultID, spec)
		registered++
	}
	if registered > 0 {
		log.Printf("[seed] registered %d domain(s) from %s", registered, dir)
	}
}

// CLIError wraps a non-zero exit reason. The thin main wrappers use it to
// decide whether to print the error or treat it as a clean shutdown.
type CLIError struct {
	Code int
	Err  error
}

func (e *CLIError) Error() string { return e.Err.Error() }

// IsCleanShutdown reports whether an error returned by Run should be treated as
// a normal exit (e.g. signal-initiated context cancellation).
func IsCleanShutdown(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}
