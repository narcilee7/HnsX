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

	"github.com/redis/go-redis/v9"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/version"
	"github.com/hnsx-io/hnsx/server/internal/audit/model"
	auditrepository "github.com/hnsx-io/hnsx/server/internal/audit/repository"
	auditservice "github.com/hnsx-io/hnsx/server/internal/audit/service"
	"github.com/hnsx-io/hnsx/server/internal/config"
	evalrepository "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	policyrepository "github.com/hnsx-io/hnsx/server/internal/policy/repository"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	"github.com/hnsx-io/hnsx/server/internal/worker/repository"
	"github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/api"
	"github.com/hnsx-io/hnsx/server/pkg/controlplane"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	hsxruntime "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/internal/telemetry"
	tracerepository "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"

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
  HNSX_REDIS_ADDR          Redis address for the session queue (e.g. 127.0.0.1:6379)
  HNSX_REDIS_PASSWORD      Redis AUTH password
  HNSX_REDIS_DB            Redis logical database number
  HNSX_REDIS_QUEUE_PREFIX  Redis key prefix for the queue (default hnsx:queue)
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

	sinks := []runtime.Sink{telemetry.NewStdoutSink()}
	if store != nil && !store.IsNoDB() {
		sinks = append(sinks, telemetry.NewDBSink(store.Pool))
	}
	if cfg.OTel.Exporter != "none" {
		sinks = append(sinks, telemetry.NewTracerSink())
	}

	// Policy + audit + trace services are backed by in-memory repositories in
	// Phase 1. Future PRs will add Postgres-backed repositories once the
	// domain/tenant mapping is stable.
	policySvc := policyservice.NewService(policyrepository.NewInMemoryRepository())
	auditSvc := auditservice.NewService(auditrepository.NewInMemoryRepository())
	traceSvc := traceservice.NewService(tracerepository.NewInMemoryRepository())
	evalSvc := evalservice.NewService(evalrepository.NewInMemoryRepository())

	traceSink := &funcSink{
		name: "trace",
		record: func(ctx context.Context, obs runtime.Observation) error {
			return traceSvc.Record(ctx, obs)
		},
	}
	sinks = append(sinks, traceSink)

	exec := hsxruntime.NewExecutor(adapter.NewNoopAdapter(), sinks...).
		WithPolicyProvider(policySvc).
		WithAuditRecorder(&auditRecorder{svc: auditSvc})

	build := api.BuildInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		Built:     version.Built,
		GoVersion: stdruntime.Version(),
	}

	// V1.1: worker pool is only enabled when a gRPC address is configured.
	// When nil, the REST API falls back to the in-process executor.
	var workerSvc *service.Service
	var workerReg *worker.Registry
	var sessionQ worker.SessionQueue
	if cfg.GRPCAddr != "" {
		if cfg.RedisEnabled() {
			rdb := redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
			defer func() {
				_ = rdb.Close()
			}()
			sessionQ = worker.NewRedisSessionQueue(rdb, cfg.Redis.QueueKeyPrefix)
			log.Printf("[hnsx-server] session queue: redis=%s prefix=%s", cfg.Redis.Addr, cfg.Redis.QueueKeyPrefix)
		} else {
			sessionQ = worker.NewSessionQueue()
			log.Printf("[hnsx-server] session queue: in-memory")
		}
		workerSvc = service.NewServiceWithQueue(repository.NewInMemoryRepository(), sessionQ)
		workerReg = workerSvc.Registry()
	}

	srv := api.NewServerWithWorkerPool(build, store, exec, workerReg, sessionQ).
		WithPolicyService(policySvc).
		WithAuditService(auditSvc).
		WithTraceService(traceSvc).
		WithEvalService(evalSvc)

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
			grpcSrv.Sched.OnObservation = func(tid tenant.ID, sessionID string, obs *pb.Observation) {
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
			grpcSrv.Sched.OnSessionStatus = func(tid tenant.ID, sessionID, state string) {
				srv.UpdateSessionState(tid, sessionID, state)
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
		if err := srv.Drain(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] api drain: %v", err)
		}
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
		s.RegisterBootstrapDomain(tenant.DefaultID, spec)
		registered++
	}
	if registered > 0 {
		log.Printf("[seed] registered %d domain(s) from %s", registered, dir)
	}
}

// auditRecorder adapts the internal audit service to the pkg/session
// AuditRecorder interface used by the executor.
type auditRecorder struct {
	svc *auditservice.Service
}

func (r *auditRecorder) Record(ctx context.Context, entry hsxruntime.AuditEntry) error {
	return r.svc.Record(ctx, &model.Entry{
		SessionID: entry.SessionID,
		DomainID:  entry.DomainID,
		Action:    entry.Action,
		Actor:     "executor",
		ActorType: model.ActorTypeSystem,
		Resource:  entry.Resource,
		Decision:  entry.Decision,
		Reason:    entry.Reason,
		Details:   entry.Details,
	})
}

// funcSink adapts a function to the runtime.Sink interface.
type funcSink struct {
	name   string
	record func(context.Context, runtime.Observation) error
}

func (s *funcSink) Name() string { return s.name }

func (s *funcSink) Record(ctx context.Context, obs runtime.Observation) error {
	return s.record(ctx, obs)
}

func (s *funcSink) Flush(context.Context) error  { return nil }
func (s *funcSink) Close(context.Context) error  { return nil }
