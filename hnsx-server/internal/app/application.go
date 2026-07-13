// Package app composes the server-side application layer. It wires together
// repositories, services, the executor, the worker queue/registry, and the
// broadcaster index. It is consumed by both the HTTP API and the gRPC control
// plane.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	approvalmodel "github.com/hnsx-io/hnsx/server/internal/approval/model"
	approvalrepo "github.com/hnsx-io/hnsx/server/internal/approval/repository"
	approvalservice "github.com/hnsx-io/hnsx/server/internal/approval/service"
	auditrepository "github.com/hnsx-io/hnsx/server/internal/audit/repository"
	auditservice "github.com/hnsx-io/hnsx/server/internal/audit/service"
	"github.com/hnsx-io/hnsx/server/internal/config"
	domainrepository "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	evalrepository "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	policyrepository "github.com/hnsx-io/hnsx/server/internal/policy/repository"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	secretcrypto "github.com/hnsx-io/hnsx/server/internal/secret/crypto"
	secretrepository "github.com/hnsx-io/hnsx/server/internal/secret/repository"
	secretservice "github.com/hnsx-io/hnsx/server/internal/secret/service"
	sessionrepository "github.com/hnsx-io/hnsx/server/internal/session/repository"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	storeservice "github.com/hnsx-io/hnsx/server/internal/store/service"
	"github.com/hnsx-io/hnsx/server/internal/telemetry"
	tracerepository "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	workerrepository "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// Executor was removed in W16+ Phase 3 (pkg/session is gone).
// Session execution now happens entirely in the Python worker; the Go
// side only orchestrates and persists.

// Application composes all server-side dependencies.
type Application struct {
	Config   *config.Config
	DB       *db.DB
	OTelProv *telemetry.Provider
	Log      *zap.Logger

	DomainService   *domainservice.Service
	SessionService  *sessionservice.Service
	WorkerService   *workerservice.Service
	PolicyService   *policyservice.Service
	AuditService    *auditservice.Service
	TraceService    *traceservice.Service
	EvalService     *evalservice.Service
	SecretService   *secretservice.Service
	ApprovalService *approvalservice.Service
	StoreService    *storeservice.Service

	State *State

	redisClient *redis.Client
}

// NewApplication wires repositories, services, and infrastructure based on cfg.
func NewApplication(ctx context.Context, cfg *config.Config, log *zap.Logger) (*Application, error) {
	if log == nil {
		return nil, errors.New("application: nil logger")
	}

	otelProv, err := telemetry.Init(ctx, telemetry.OTelOptions{
		ServiceName:  cfg.OTel.ServiceName,
		Exporter:     cfg.OTel.Exporter,
		OTLPEndpoint: cfg.OTel.OTLPEndpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("otel: %w", err)
	}

	store, err := openStore(ctx, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if cfg.SQLiteEnabled() {
		// Apply the SQLite-specific minimum schema (tenants + domains +
		// domain_versions) directly via Exec — the existing goose set is
		// Postgres-only. Other repos stay Postgres-bound; v1.1 will port
		// the remaining schema.
		if err := db.EnsureSQLiteSchema(store); err != nil {
			return nil, fmt.Errorf("db: sqlite schema: %w", err)
		}
		log.Info("sqlite schema applied (tenants + domains + domain_versions)")
	} else if err := db.Migrate(ctx, store, cfg.MigrationsDir); err != nil {
		return nil, fmt.Errorf("db: migrate: %w", err)
	} else {
		log.Info("migrations applied", zap.String("dir", cfg.MigrationsDir))
	}

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
	approvalRepo := approvalrepo.NewPostgresRepository(store)

	// Services.
	domainSvc := domainservice.NewService(domainRepo)
	sessionSvc := sessionservice.NewService(sessionRepo)
	policySvc := policyservice.NewService(policyRepo)
	auditSvc := auditservice.NewService(auditRepo)
	traceSvc := traceservice.NewService(traceRepo)
	evalSvc := evalservice.NewService(evalRepo)
	// Secret encryption at rest is fail-fast: HNSX_SECRET_KEY must be set
	// before the control plane boots. Server refuses to start without it
	// rather than silently downgrading to plaintext.
	secretCipher, err := secretcrypto.New()
	if err != nil {
		return nil, fmt.Errorf("secret cipher: %w (set HNSX_SECRET_KEY, min 16 chars)", err)
	}
	log.Info("secret store: encryption enabled", zap.String("alg", "AES-256-GCM"))
	secretSvc := secretservice.NewService(secretRepo, secretCipher)
	storeSvc := storeservice.NewService(store)
	approvalSvc := approvalservice.NewService(approvalRepo, approvalStateBroadcaster{state: appState})

	// Sinks.
	sinks := []domain.Sink{
		telemetry.NewStdoutSink(),
		telemetry.NewDBSink(store.GormDB),
	}
	if cfg.OTel.Exporter != "none" {
		sinks = append(sinks, telemetry.NewTracerSink()) //nolint:typecheck // W16+ Phase 3 migration
	}
	traceSink := &funcSink{
		name: "trace",
		record: func(ctx context.Context, obs domain.Observation) error {
			return traceSvc.Record(ctx, obs)
		},
	}
	sinks = append(sinks, traceSink)

	// Executor removed in W16+ Phase 3 — session execution is now fully
	// delegated to the Python worker. The Go side orchestrates and
	// persists; policy/approval/audit hooks are wired in the worker.

	// Worker pool is only enabled when a gRPC address is configured.
	var workerSvc *workerservice.Service
	var rdb *redis.Client
	if cfg.GRPCAddr != "" {
		var sessionQ worker.SessionQueue
		if cfg.RedisEnabled() {
			rdb = redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
			sessionQ = worker.NewRedisSessionQueue(rdb, cfg.Redis.QueueKeyPrefix)
			log.Info("session queue: redis",
				zap.String("addr", cfg.Redis.Addr),
				zap.String("prefix", cfg.Redis.QueueKeyPrefix))
		} else {
			sessionQ = worker.NewSessionQueue()
			log.Info("session queue: in-memory")
		}
		workerSvc = workerservice.NewServiceWithQueue(workerRepo, sessionQ)
	}

	return &Application{
		Config:          cfg,
		DB:              store,
		OTelProv:        otelProv,
		Log:             log,
		DomainService:   domainSvc,
		SessionService:  sessionSvc,
		WorkerService:   workerSvc,
		ApprovalService: approvalSvc,
		PolicyService:   policySvc,
		AuditService:    auditSvc,
		TraceService:    traceSvc,
		EvalService:     evalSvc,
		SecretService:   secretSvc,
		StoreService:    storeSvc,
		State:           appState,
		redisClient:     rdb,
	}, nil
}

// Close cleans up resources held by the application.
func (a *Application) Close(ctx context.Context) error {
	if a.WorkerService != nil {
		a.WorkerService.Close()
	}
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

// approvalServiceGateAdapter removed in W16+ Phase 3 (Executor gone).
// Approval gating now lives in the Python worker; the Go side only
// persists state via the internal approval service.

// approvalStateBroadcaster publishes approval lifecycle events as session
// observations so SSE consumers (Console) see them in real time.
type approvalStateBroadcaster struct {
	state *State
}

func (b approvalStateBroadcaster) PublishApproval(event string, a *approvalmodel.Approval) {
	if b.state == nil || a == nil {
		return
	}
	now := time.Now()
	base := domain.Observation{
		SessionID: a.SessionID,
		DomainID:  a.DomainID,
		Timestamp: now,
	}
	switch event {
	case "approval_required":
		b.state.PublishObservation(a.SessionID, domain.Observation{
			Kind:      "approval_required",
			SessionID: a.SessionID,
			DomainID:  a.DomainID,
			Timestamp: now,
			Payload: map[string]any{
				"id":           a.ID,
				"action":       a.Action,
				"resource":     a.Resource,
				"risk_level":   a.RiskLevel,
				"context":      a.Context,
				"requested_by": a.RequestedBy,
			},
		})
		b.state.PublishObservation(a.SessionID, domain.Observation{
			Kind:      "state",
			SessionID: a.SessionID,
			DomainID:  a.DomainID,
			Timestamp: now,
			Payload:   map[string]any{"state": "paused"},
		})
	case "approval_resolved":
		b.state.PublishObservation(a.SessionID, domain.Observation{
			Kind:      "approval_resolved",
			SessionID: a.SessionID,
			DomainID:  a.DomainID,
			Timestamp: now,
			Payload: map[string]any{
				"id":          a.ID,
				"status":      a.Status,
				"reviewed_by": a.ReviewedBy,
				"comment":     a.Comment,
			},
		})
		b.state.PublishObservation(a.SessionID, domain.Observation{
			Kind:      "state",
			SessionID: a.SessionID,
			DomainID:  a.DomainID,
			Timestamp: now,
			Payload:   map[string]any{"state": "running"},
		})
		b.state.PublishObservation(a.SessionID, domain.Observation{
			Kind:      "session_resumed",
			SessionID: a.SessionID,
			DomainID:  a.DomainID,
			Timestamp: now,
			Payload: map[string]any{
				"reason":      "approval_resolved",
				"approval_id": a.ID,
			},
		})
	}
	_ = base
}

// auditRecorder removed in W16+ Phase 3 (Executor gone).
// Audit recording now lives in the Python worker; the Go side only
// persists via its own internal audit service.

// funcSink adapts a function to the runtime.Sink interface.
type funcSink struct {
	name   string
	record func(context.Context, domain.Observation) error
}

// openStore picks Postgres when HNSX_DATABASE_URL is set, otherwise the
// embedded SQLite file at <DaemonDataDir>/hnsx.db (parent dir auto-created).
func openStore(ctx context.Context, cfg *config.Config, log *zap.Logger) (*db.DB, error) {
	if cfg.PostgresEnabled() {
		return db.Open(ctx, cfg.DatabaseURL)
	}
	sqlitePath := cfg.SQLitePath
	if sqlitePath == "" {
		sqlitePath = filepath.Join(cfg.DaemonDataDir, "hnsx.db")
	}
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		return nil, fmt.Errorf("daemon: mkdir %s: %w", filepath.Dir(sqlitePath), err)
	}
	log.Info("daemon mode: using embedded SQLite",
		zap.String("path", sqlitePath),
		zap.String("data_dir", cfg.DaemonDataDir))
	store, err := db.OpenSQLite(sqlitePath)
	if err != nil {
		return nil, err
	}
	if err := ensureSecretKey(cfg.DaemonDataDir, log); err != nil {
		return nil, fmt.Errorf("daemon: secret key: %w", err)
	}
	return store, nil
}

// ensureSecretKey writes a random 32-byte hex key to <dataDir>/secret.key
// on first boot and exports HNSX_SECRET_KEY for the rest of the process.
// Existing users keep their key so encrypted secrets stay decryptable.
func ensureSecretKey(dataDir string, log *zap.Logger) error {
	keyPath := filepath.Join(dataDir, "secret.key")
	if _, err := os.Stat(keyPath); err == nil {
		b, readErr := os.ReadFile(keyPath)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", keyPath, readErr)
		}
		_ = os.Setenv("HNSX_SECRET_KEY", strings.TrimSpace(string(b)))
		return nil
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("rand: %w", err)
	}
	hexed := hex.EncodeToString(key)
	if err := os.WriteFile(keyPath, []byte(hexed), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", keyPath, err)
	}
	_ = os.Setenv("HNSX_SECRET_KEY", hexed)
	log.Info("auto-generated HNSX_SECRET_KEY",
		zap.String("path", keyPath),
		zap.String("action", "BACK THIS UP — losing this file means losing access to encrypted secrets"))
	return nil
}

func (s *funcSink) Name() string { return s.name }

func (s *funcSink) Record(ctx context.Context, obs domain.Observation) error {
	return s.record(ctx, obs)
}

func (s *funcSink) Flush(context.Context) error { return nil }
func (s *funcSink) Close(context.Context) error { return nil }
