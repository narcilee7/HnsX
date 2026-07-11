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
	"os"
	stdruntime "runtime"
	"time"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/logger"
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
	Log         *zap.Logger
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

	log, err := logger.New(cfg.Log.Level)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	ctx := context.Background()
	application, err := app.NewApplication(ctx, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("application: %w", err)
	}

	build := api.BuildInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		Built:     version.Built,
		GoVersion: stdruntime.Version(),
	}
	apiServer := api.NewServer(build, application)
	apiServer.TemplatesIndexPath = cfg.TemplatesIndexPath

	connectSrv := controlplane.NewConnectServer(application)
	apiServer.WithConnectHandler(connectSrv.Handler())

	if *seedFrom != "" {
		seedFromDir(log, apiServer, *seedFrom)
	}

	s := &Server{
		Config:      cfg,
		Application: application,
		APIServer:   apiServer,
		Log:         log,
	}

	if cfg.GRPCAddr != "" {
		grpcSrv := controlplane.NewServer(cfg.GRPCAddr).WithWorkerService(application.WorkerService)
		if grpcSrv.Sched != nil {
			grpcSrv.Sched.OnObservation = func(tid tenant.ID, sessionID string, obs *pb.Observation) {
				payload := map[string]any{}
				if obs.GetPayload() != "" {
					_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
				}
				metadata := map[string]any{}
				if obs.GetMetadata() != "" {
					_ = json.Unmarshal([]byte(obs.GetMetadata()), &metadata)
				}
				ro := runtime.Observation{
					Kind:      obs.GetKind(),
					SessionID: obs.GetSessionId(),
					DomainID:  obs.GetDomainId(),
					StepID:    obs.GetStepId(),
					AgentID:   obs.GetAgentId(),
					ParentID:  obs.GetParentId(),
					TraceID:   obs.GetTraceId(),
					Payload:   payload,
					Metadata:  metadata,
					Timestamp: time.UnixMilli(obs.GetCreatedAtMs()),
				}
				apiServer.PublishObservation(sessionID, ro)
				if apiServer.TraceService != nil {
					_ = apiServer.TraceService.Record(context.Background(), ro)
				}
				if application.Executor != nil {
					application.Executor.ForwardObservation(context.Background(), ro)
				}
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

	s.Log.Info("hnsx-server listening",
		zap.String("http", s.Config.HTTPAddr),
		zap.String("grpc", s.Config.GRPCAddr))
	s.Log.Info("hnsx-server build info",
		zap.String("version", s.APIServer.BuildInfo.Version),
		zap.String("commit", s.APIServer.BuildInfo.Commit))
	s.Log.Info("hnsx-server runtime",
		zap.String("otel", s.Config.OTel.Exporter),
		zap.String("go", s.APIServer.BuildInfo.GoVersion))

	select {
	case <-ctx.Done():
		s.Log.Info("hnsx-server shutting down")
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
		s.Log.Warn("api drain error", zap.Error(err))
	}
	if err := s.APIServer.Shutdown(shutdownCtx); err != nil {
		s.Log.Warn("api shutdown error", zap.Error(err))
	}
	if s.GRPCServer != nil {
		if err := s.GRPCServer.Shutdown(shutdownCtx); err != nil {
			s.Log.Warn("grpc shutdown error", zap.Error(err))
		}
	}
	if err := s.Application.Close(shutdownCtx); err != nil {
		s.Log.Warn("application close error", zap.Error(err))
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
			s.Log.Info("evicted stale workers", zap.Int("count", len(evicted)), zap.Strings("workers", evicted))
			if s.GRPCServer == nil || s.GRPCServer.Sched == nil {
				continue
			}
			for _, wid := range evicted {
				if requeued := s.GRPCServer.Sched.RequeueSessions(wid); len(requeued) > 0 {
					s.Log.Info("requeued sessions from worker", zap.Int("count", len(requeued)), zap.String("worker_id", wid))
				}
			}
		}
	}
}

// seedFromDir walks the given directory and registers every v2 DomainSpec YAML
// against the API server. It is an explicit operator action (--seed-from) so
// production deployments never implicitly pull in development fixtures.
func seedFromDir(log *zap.Logger, s *api.Server, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warn("seed cannot read directory", zap.String("dir", dir), zap.Error(err))
		return
	}
	registered := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := fmt.Sprintf("%s/%s/domain.yaml", dir, e.Name())
		ds, err := spec.LoadFile(path)
		if err != nil {
			log.Warn("seed skip file", zap.String("path", path), zap.Error(err))
			continue
		}
		s.RegisterBootstrapDomain(tenant.DefaultID, ds)
		registered++
	}
	if registered > 0 {
		log.Info("seed registered domains", zap.Int("count", registered), zap.String("dir", dir))
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
