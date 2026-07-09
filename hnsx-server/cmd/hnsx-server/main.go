// hnsx-server is the HnsX control plane daemon. It hosts the HTTP/REST API
// and (in future PRs) the gRPC control plane. Phase 1 focuses on:
//
//   - Loading configuration (env+yaml).
//   - Optionally connecting Postgres and running migrations.
//   - Initialising OTel (stdout / otlp / none).
//   - Starting the HTTP API + SSE handler on the configured address.
//
// Usage:
//
//	hnsx-server server [--config <path>]
//	hnsx-server version
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	stdruntime "runtime"
	"syscall"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/version"
	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	"github.com/hnsx-io/hnsx/server/internal/worker/repository"
	"github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/api"
	"github.com/hnsx-io/hnsx/server/pkg/controlplane"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	hsxruntime "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/telemetry"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		os.Exit(cmdServer(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version.String())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`hnsx-server — HnsX Control Plane

Usage:
  hnsx-server server [--config <path>]
  hnsx-server version

Environment:
  HNSX_HTTP_ADDR           Listen address (default 127.0.0.1:50051)
  HNSX_DATABASE_URL        Postgres connection string
  HNSX_MIGRATIONS_DIR      SQL migrations directory
  HNSX_OTEL_EXPORTER       stdout | otlp | none
  HNSX_OTEL_OTLP_ENDPOINT  OTLP gRPC endpoint (default 127.0.0.1:4317)
  HNSX_OTEL_SERVICE_NAME   service.name attribute
  HNSX_LOG_LEVEL           debug | info | warn | error
`)
}

func cmdServer(args []string) int {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "optional path to YAML config")
	seedFrom := fs.String("seed-from", "", "optional directory of v2 DomainSpec YAMLs to register on boot (development only; production deployments register via POST /api/v1/domains)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	otelProv, err := telemetry.Init(ctx, telemetry.OTelOptions{
		ServiceName:  cfg.OTel.ServiceName,
		Exporter:     cfg.OTel.Exporter,
		OTLPEndpoint: cfg.OTel.OTLPEndpoint,
	})
	if err != nil {
		log.Fatalf("otel: %v", err)
	}

	var store *db.DB
	if cfg.PostgresEnabled() {
		store, err = db.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Printf("[hnsx-server] WARNING: database unavailable: %v", err)
			store = db.NoDB()
		} else {
			defer store.Close()
			if err := db.Migrate(ctx, store.SQL, cfg.MigrationsDir); err != nil {
				log.Fatalf("migrate: %v", err)
			}
			log.Printf("[hnsx-server] migrations applied from %s", cfg.MigrationsDir)
		}
	} else {
		store = db.NoDB()
		log.Printf("[hnsx-server] running in no-db mode (set HNSX_DATABASE_URL to enable)")
	}

	sinks := []telemetry.Sink{telemetry.NewStdoutSink()}
	if store != nil && !store.IsNoDB() {
		sinks = append(sinks, telemetry.NewDBSink(store.Pool))
	}
	if cfg.OTel.Exporter != "none" {
		sinks = append(sinks, telemetry.NewTracerSink())
	}

	exec := hsxruntime.NewExecutor(adapter.NewNoopAdapter(), sinks...)

	build := api.BuildInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		Built:     version.Built,
		GoVersion: stdruntime.Version(),
	}

	// V1.1: worker pool is only enabled when a gRPC address is configured.
	// When nil, the REST API falls back to the in-process executor.
	var workerSvc *service.Service
	if cfg.GRPCAddr != "" {
		workerSvc = service.NewService(repository.NewInMemoryRepository())
	}

	var workerReg *worker.Registry
	var sessionQ *worker.SessionQueue
	if workerSvc != nil {
		workerReg = workerSvc.Registry()
		sessionQ = workerSvc.Queue()
	}
	srv := api.NewServerWithWorkerPool(build, store, exec, workerReg, sessionQ)

	if *seedFrom != "" {
		seedFromDir(srv, *seedFrom)
	}

	// Stale-worker GC: every 30s, evict workers that haven't heartbeat
	// in over 60s and log how many were reaped. Only runs when worker pool
	// is enabled.
	stopGC := make(chan struct{})
	if workerSvc != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopGC:
					return
				case <-ticker.C:
					if evicted := workerSvc.EvictStale(60 * time.Second); len(evicted) > 0 {
						log.Printf("[hnsx-server] evicted %d stale worker(s): %v", len(evicted), evicted)
					}
				}
			}
		}()
	}

	var grpcSrv *controlplane.Server
	if cfg.GRPCAddr != "" {
		grpcSrv = controlplane.NewServer(cfg.GRPCAddr).WithWorkerServices(workerReg, sessionQ)
		if grpcSrv.Sched != nil {
			grpcSrv.Sched.OnObservation = func(sessionID string, obs *pb.Observation) {
				payload := map[string]any{}
				if obs.GetPayload() != "" {
					_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
				}
				srv.PublishObservation(sessionID, runtime.Observation{
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
			grpcSrv.Sched.OnSessionStatus = func(sessionID, state string) {
				srv.UpdateSessionState(sessionID, state)
			}
		}
	}

	log.Printf("[hnsx-server] listening on http=%s grpc=%s", cfg.HTTPAddr, cfg.GRPCAddr)
	log.Printf("[hnsx-server] version=%s commit=%s", build.Version, build.Commit)
	log.Printf("[hnsx-server] otel=%s build=%s", cfg.OTel.Exporter, build.GoVersion)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Listen(cfg.HTTPAddr)
	}()

	var grpcErr chan error
	if grpcSrv != nil {
		grpcErr = make(chan error, 1)
		go func() {
			grpcErr <- grpcSrv.ListenAndServe(ctx)
		}()
	}

	select {
	case <-ctx.Done():
		log.Println("[hnsx-server] shutting down")
		if workerReg != nil {
			close(stopGC)
		}
		shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] api shutdown: %v", err)
		}
		if otelProv != nil {
			if err := otelProv.Shutdown(shutdownCtx); err != nil {
				log.Printf("[hnsx-server] otel shutdown: %v", err)
			}
		}
		return 0
	case err := <-serveErr:
		log.Fatalf("http: %v", err)
		return 1
	case err := <-grpcErr:
		log.Fatalf("grpc: %v", err)
		return 1
	}
}

// seedFromDir walks the given directory and registers every v2 DomainSpec YAML
// against the API server. It is an explicit operator action (--seed-from) so
// production deployments never implicitly pull in development fixtures.
//
// Each subdirectory of dir is expected to contain a single `domain.yaml`. Any
// YAML that fails validation is logged and skipped — the server still boots.
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
		s.RegisterBootstrapDomain(spec)
		registered++
	}
	if registered > 0 {
		log.Printf("[seed] registered %d domain(s) from %s", registered, dir)
	}
}
