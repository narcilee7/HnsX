// Package app composes the server-side application layer. It wires together
// repositories, services, the executor, the worker queue/registry, and the
// broadcaster index. It is consumed by both the HTTP API and the gRPC control
// plane.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
	auditrepository "github.com/hnsx-io/hnsx/server/internal/audit/repository"
	auditservice "github.com/hnsx-io/hnsx/server/internal/audit/service"
	"github.com/hnsx-io/hnsx/server/internal/config"
	domainrepository "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	evalrepository "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	policyrepository "github.com/hnsx-io/hnsx/server/internal/policy/repository"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	secretrepository "github.com/hnsx-io/hnsx/server/internal/secret/repository"
	secretservice "github.com/hnsx-io/hnsx/server/internal/secret/service"
	sessionrepository "github.com/hnsx-io/hnsx/server/internal/session/repository"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/telemetry"
	tracerepository "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	workerrepository "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
)

// Application composes all server-side dependencies.
type Application struct {
	Config   *config.Config
	DB       *db.DB
	OTelProv *telemetry.Provider

	DomainService  *domainservice.Service
	SessionService *sessionservice.Service
	WorkerService  *workerservice.Service
	PolicyService  *policyservice.Service
	AuditService   *auditservice.Service
	TraceService   *traceservice.Service
	EvalService    *evalservice.Service
	SecretService  *secretservice.Service

	Executor       *pkgexecutor.Executor
	WorkerRegistry *worker.Registry
	SessionQueue   worker.SessionQueue

	State *State

	redisClient *redis.Client
}

// NewApplication wires repositories, services, and infrastructure based on cfg.
func NewApplication(ctx context.Context, cfg *config.Config) (*Application, error) {
	otelProv, err := telemetry.Init(ctx, telemetry.OTelOptions{
		ServiceName:  cfg.OTel.ServiceName,
		Exporter:     cfg.OTel.Exporter,
		OTLPEndpoint: cfg.OTel.OTLPEndpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("otel: %w", err)
	}

	if !cfg.PostgresEnabled() {
		return nil, errors.New("postgres is required: set HNSX_DATABASE_URL")
	}

	store, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := db.Migrate(ctx, store.SQL, cfg.MigrationsDir); err != nil {
		return nil, fmt.Errorf("db: migrate: %w", err)
	}
	log.Printf("[hnsx-server] migrations applied from %s", cfg.MigrationsDir)

	appState := NewState()

	// Repositories: GORM/Postgres only. InMemory implementations remain in
	// repository packages as test helpers but are never used at runtime.
	domainRepo := domainrepository.NewPostgresRepository(store)
	sessionRepo := sessionrepository.NewPostgresRepository(store)
	workerRepo := workerrepository.NewPostgresRepository(store)
	auditRepo := auditrepository.NewPostgresRepository(store)
	traceRepo := tracerepository.NewPostgresRepository(store)
	evalRepo := evalrepository.NewPostgresRepository(store)
	policyRepo := policyrepository.NewPostgresRepository(store)
	secretRepo := secretrepository.NewPostgresRepository(store)

	// Services.
	domainSvc := domainservice.NewService(domainRepo)
	sessionSvc := sessionservice.NewService(sessionRepo)
	policySvc := policyservice.NewService(policyRepo)
	auditSvc := auditservice.NewService(auditRepo)
	traceSvc := traceservice.NewService(traceRepo)
	evalSvc := evalservice.NewService(evalRepo)
	secretSvc := secretservice.NewService(secretRepo)

	// Sinks.
	sinks := []runtime.Sink{
		telemetry.NewStdoutSink(),
		telemetry.NewDBSink(store.Pool),
	}
	if cfg.OTel.Exporter != "none" {
		sinks = append(sinks, telemetry.NewTracerSink())
	}
	traceSink := &funcSink{
		name: "trace",
		record: func(ctx context.Context, obs runtime.Observation) error {
			return traceSvc.Record(ctx, obs)
		},
	}
	sinks = append(sinks, traceSink)

	// Executor.
	exec := pkgexecutor.NewExecutor(adapter.NewNoopAdapter(), sinks...).
		WithPolicyProvider(policySvc).
		WithAuditRecorder(&auditRecorder{svc: auditSvc})

	// Worker pool is only enabled when a gRPC address is configured.
	var workerSvc *workerservice.Service
	var workerReg *worker.Registry
	var sessionQ worker.SessionQueue
	var rdb *redis.Client
	if cfg.GRPCAddr != "" {
		if cfg.RedisEnabled() {
			rdb = redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
			sessionQ = worker.NewRedisSessionQueue(rdb, cfg.Redis.QueueKeyPrefix)
			log.Printf("[hnsx-server] session queue: redis=%s prefix=%s", cfg.Redis.Addr, cfg.Redis.QueueKeyPrefix)
		} else {
			sessionQ = worker.NewSessionQueue()
			log.Printf("[hnsx-server] session queue: in-memory")
		}
		workerSvc = workerservice.NewServiceWithQueue(workerRepo, sessionQ)
		workerReg = workerSvc.Registry()
	}

	return &Application{
		Config:         cfg,
		DB:             store,
		OTelProv:       otelProv,
		DomainService:  domainSvc,
		SessionService: sessionSvc,
		WorkerService:  workerSvc,
		PolicyService:  policySvc,
		AuditService:   auditSvc,
		TraceService:   traceSvc,
		EvalService:    evalSvc,
		SecretService:  secretSvc,
		Executor:       exec,
		WorkerRegistry: workerReg,
		SessionQueue:   sessionQ,
		State:          appState,
		redisClient:    rdb,
	}, nil
}

// Close cleans up resources held by the application.
func (a *Application) Close(ctx context.Context) error {
	if a.redisClient != nil {
		_ = a.redisClient.Close()
	}
	if a.DB != nil {
		a.DB.Close()
	}
	if a.OTelProv != nil {
		return a.OTelProv.Shutdown(ctx)
	}
	return nil
}

// auditRecorder adapts the internal audit service to the pkg/session
// AuditRecorder interface used by the executor.
type auditRecorder struct {
	svc *auditservice.Service
}

func (r *auditRecorder) Record(ctx context.Context, entry pkgexecutor.AuditEntry) error {
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

func (s *funcSink) Flush(context.Context) error { return nil }
func (s *funcSink) Close(context.Context) error { return nil }
