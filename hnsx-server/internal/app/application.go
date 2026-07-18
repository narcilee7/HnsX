// Package app is the composition root for hnsxd. It is the only package
// allowed to import across all layers (domain / service / infra / api /
// grpc / ws / cli / server) and to wire concrete implementations to ports.
//
// The split:
//
//	domain.*   — pure entities + ports (no infra imports)
//	service.*  — orchestrates domain via ports (no concrete infra imports)
//	infra.*    — implements domain ports
//	api/grpc/ws/cli — transports, depend only on service layer
//	app        — wires the above; nothing else is allowed to do so
//
// New resources are added by:
//
//  1. declaring the entity + repo port in internal/domain/<resource>/
//  2. implementing the repo in internal/infra/db/postgres/
//  3. adding commands/queries in internal/service/<resource>/
//  4. exposing transport via internal/api/<resource>/{router,handler,dto}
//
// app.New wires all four steps together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/api/router"
	wshandler "github.com/hnsx-io/hnsx/server/internal/api/handler/ws"
	agenthandler "github.com/hnsx-io/hnsx/server/internal/api/handler/agent"
	approvalhandler "github.com/hnsx-io/hnsx/server/internal/api/handler/approval"
	daemonhandler "github.com/hnsx-io/hnsx/server/internal/api/handler/daemon"
	evalhandler "github.com/hnsx-io/hnsx/server/internal/api/handler/eval"
	harnesshandler "github.com/hnsx-io/hnsx/server/internal/api/handler/harness"
	issuehandler "github.com/hnsx-io/hnsx/server/internal/api/handler/issue"
	policyhandler "github.com/hnsx-io/hnsx/server/internal/api/handler/policy"
	squadhandler "github.com/hnsx-io/hnsx/server/internal/api/handler/squad"
	workspacehandler "github.com/hnsx-io/hnsx/server/internal/api/handler/workspace"
	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	agentinfra "github.com/hnsx-io/hnsx/server/internal/infra/agentruntime" // concrete backends
	"github.com/hnsx-io/hnsx/server/internal/infra/db/postgres"
	agentsvc "github.com/hnsx-io/hnsx/server/internal/service/agent"
	approvalsvc "github.com/hnsx-io/hnsx/server/internal/service/approval"
	daemonsvc "github.com/hnsx-io/hnsx/server/internal/service/daemon"
	daemonruntime "github.com/hnsx-io/hnsx/server/internal/service/daemon_runtime"
	evalsvc "github.com/hnsx-io/hnsx/server/internal/service/eval"
	harnesssvc "github.com/hnsx-io/hnsx/server/internal/service/harness"
	issuesvc "github.com/hnsx-io/hnsx/server/internal/service/issue"
	policysvc "github.com/hnsx-io/hnsx/server/internal/service/policy"
	squadsvc "github.com/hnsx-io/hnsx/server/internal/service/squad"
	workspacesvc "github.com/hnsx-io/hnsx/server/internal/service/workspace"
	wsclient "github.com/hnsx-io/hnsx/server/internal/ws"
	"github.com/hnsx-io/hnsx/server/internal/ws/handler"
)

// wsURLFromHTTPAddr converts the server's HTTP bind address to a
// WebSocket URL pointing at /ws/daemon. Strips the leading ":" for
// port-only binds and prepends "ws://".
func wsURLFromHTTPAddr(addr string) string {
	addr = strings.TrimPrefix(addr, ":")
	if !strings.Contains(addr, "://") {
		addr = "ws://127.0.0.1:" + addr
	}
	return strings.TrimRight(addr, "/") + "/ws/daemon"
}

// policyLookup is the daemon_runtime.PolicyLookup implementation that
// returns the workspace's first Policy. R3.x swaps this for a per-harness
// binding lookup.
type policyLookup struct {
	repo *postgres.PolicyRepo
}

func (p *policyLookup) FirstPolicyForWorkspace(ctx context.Context, workspaceID string) (*policy.Policy, error) {
	list, err := p.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// Config holds runtime configuration for hnsxd. Loaded from
// HNSX_* environment variables and an optional YAML file.
type Config struct {
	// HTTPAddr is the bind address for the HTTP+WS server (default ":8080").
	HTTPAddr string `yaml:"http_addr" env:"HNSX_HTTP_ADDR"`
	// PostgresDSN is the connection string for the HnsX Postgres database.
	PostgresDSN string `yaml:"postgres_dsn" env:"HNSX_POSTGRES_DSN"`
	// LogLevel is one of debug, info, warn, error (default "info").
	LogLevel string `yaml:"log_level" env:"HNSX_LOG_LEVEL"`
	// ClaudeExecutable overrides the `claude` CLI lookup path (default: PATH).
	ClaudeExecutable string `yaml:"claude_executable" env:"HNSX_CLAUDE_EXECUTABLE"`
}

// LoadConfig reads configuration from the given file (if non-empty) and
// environment variables. File values are overridden by HNSX_* env vars.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		HTTPAddr:         getEnv("HNSX_HTTP_ADDR", ":8080"),
		PostgresDSN:      os.Getenv("HNSX_POSTGRES_DSN"),
		LogLevel:         getEnv("HNSX_LOG_LEVEL", "info"),
		ClaudeExecutable: getEnv("HNSX_CLAUDE_EXECUTABLE", ""),
	}
	if path != "" {
		return nil, fmt.Errorf("config file loading not yet implemented: %s", path)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Application is the wired-up runtime. Held by main; Close() releases
// every resource opened during New.
type Application struct {
	cfg      *Config
	logger   *slog.Logger
	Backends agentruntime.Registry

	// Resource services exposed for CLI / WS transports.
	WorkspaceSvc *workspacesvc.Service
	AgentSvc     *agentsvc.Service
	IssueSvc     *issuesvc.Service
	SquadSvc     *squadsvc.Service
	DaemonSvc    *daemonsvc.Service
	HarnessSvc   *harnesssvc.Service
	PolicySvc    *policysvc.Service
	EvalSvc      *evalsvc.Service
	ApprovalSvc  *approvalsvc.Service

	// DB pool + handlers. Available so WS layer (R1.9) and tests can use them.
	DB       *postgres.DB
	Handlers router.Deps

	// DaemonRuntime is the long-running worker loop. Lazily constructed
	// only when DB is available; nil otherwise.
	DaemonRuntime *daemonruntime.Service

	// HTTP server lifecycle.
	httpServer *http.Server
}

// New constructs the application: loads config, opens DB pool, wires
// repos -> services -> handlers -> router.
//
// The Postgres pool is optional: if cfg.PostgresDSN is empty, the
// application boots without DB and the HTTP server responds with 500 on
// any endpoint that needs persistence. This keeps `hnsxd backends list`
// (and other CLI subcommands) usable without a running database.
func New(ctx context.Context, cfg *Config) (*Application, error) {
	if cfg == nil {
		return nil, errors.New("app: nil config")
	}

	logger, err := newLogger(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	app := &Application{
		cfg:    cfg,
		logger: logger,
	}

	// 1. Agent runtime registry (no DB needed).
	registry := agentinfra.NewDefaultRegistry(logger)
	// Honor HNSX_CLAUDE_EXECUTABLE: re-register the claude backend with
	// the override path so the rest of the registry stays untouched.
	if cfg.ClaudeExecutable != "" {
		registry.Register(agentinfra.NewClaudeBackend(agentinfra.NewClaudeRunner(cfg.ClaudeExecutable, logger)))
	}
	app.Backends = registry
	logger.Info("app: agent backends registered",
		"backends", strings.Join(registry.List(), ", "),
	)

	// 2. Database pool (optional).
	if cfg.PostgresDSN != "" {
		// Postgres package expects a *zap.Logger. We pass a no-op one and
		// rely on slog at the application boundary; GORM slow queries are
		// still logged via slog inside the postgres package's warn path.
		db, err := postgres.Open(ctx, postgres.Config{
			DSN:        cfg.PostgresDSN,
			LogQueries: false,
		}, zap.NewNop())
		if err != nil {
			return nil, fmt.Errorf("app: postgres: %w", err)
		}
		app.DB = db
		logger.Info("app: postgres ready")
	} else {
		logger.Warn("app: postgres DSN not set; DB-backed endpoints will fail")
	}

	// 3. Wire repos -> services -> handlers (only if DB is available).
	if app.DB != nil {
		workspaceRepo := postgres.NewWorkspaceRepo(app.DB)
		agentRepo := postgres.NewAgentRepo(app.DB)
		issueRepo := postgres.NewIssueRepo(app.DB)
		squadRepo := postgres.NewSquadRepo(app.DB)
		daemonRepo := postgres.NewDaemonRepo(app.DB)

		app.WorkspaceSvc = workspacesvc.New(workspaceRepo)
		app.AgentSvc = agentsvc.New(agentRepo)
		app.IssueSvc = issuesvc.New(issueRepo)
		app.SquadSvc = squadsvc.New(squadRepo)
		app.DaemonSvc = daemonsvc.New(daemonRepo)
		app.HarnessSvc = harnesssvc.New(postgres.NewHarnessRepo(app.DB))
		app.PolicySvc = policysvc.New(postgres.NewPolicyRepo(app.DB))
		app.EvalSvc = evalsvc.New(
			postgres.NewEvalSetRepo(app.DB),
			postgres.NewEvalRunRepo(app.DB),
			postgres.NewObservationSink(app.DB),
			logger,
		)
		app.ApprovalSvc = approvalsvc.New(postgres.NewApprovalRepo(app.DB))

		app.Handlers = router.Deps{
			Workspace: workspacehandler.New(app.WorkspaceSvc),
			Issue:     issuehandler.New(app.IssueSvc),
			Agent:     agenthandler.New(app.AgentSvc),
			Squad:     squadhandler.New(app.SquadSvc),
			Daemon:    daemonhandler.New(app.DaemonSvc),
			Harness:   harnesshandler.New(app.HarnessSvc),
			Policy:    policyhandler.New(app.PolicySvc),
			Eval:      evalhandler.New(app.EvalSvc),
			Approval:  approvalhandler.New(app.ApprovalSvc),
		}

		// Daemon runtime: pulls assigned issues, spawns the agent backend,
		// streams observations. Lazily wired only when DB is available.
		sink := postgres.NewObservationSink(app.DB)
		setRepo := postgres.NewEvalSetRepo(app.DB)
		runRepo := postgres.NewEvalRunRepo(app.DB)
		evalSvc := evalsvc.New(setRepo, runRepo, sink, logger)
		policyRepo := postgres.NewPolicyRepo(app.DB)
		approvalRepo := postgres.NewApprovalRepo(app.DB)
		gate := approvalsvc.NewGate(approvalsvc.GateConfig{
			Repo:   approvalRepo,
			Logger: logger,
		})
		engine := policysvc.NewEngine()

		// WS client: the daemon runtime talks to the server over
		// /ws/daemon instead of sharing the Postgres pool. Built even
		// for the in-process case so the runtime uses the same code
		// path whether the daemon is local or remote.
		wsURL := wsURLFromHTTPAddr(cfg.HTTPAddr)
		wsClient := wsclient.NewClient(wsURL)
		dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := wsClient.Connect(dialCtx); err != nil {
			logger.Warn("app: ws client connect failed (daemon will fall back to direct DB)",
				"url", wsURL, "err", err)
		} else {
			logger.Info("app: ws client connected to daemon endpoint", "url", wsURL)
		}
		dialCancel()

		app.DaemonRuntime = daemonruntime.New(daemonruntime.Config{
			Issues:   app.IssueSvc,
			Agents:   app.AgentSvc,
			Registry: app.Backends,
			Sink:     sink,
			WS:       wsClient,
			Eval:     evalAutoRunner{evalSvc},
			Policies: &policyLookup{repo: policyRepo},
			Gate:     gate,
			Engine:   engine,
			Logger:   logger,
		})
	}

	// 4. HTTP server lifecycle.
	engine := router.New(app.Handlers)

	// 4a. /ws/daemon endpoint. Daemons connect here for claim/observation
	//     round-trips; the WS handler is wired with the same services
	//     the HTTP handlers use, so the source of truth stays in one place.
	if app.DB != nil {
		wsBridge := &wsServiceAdapter{
			issues: &issueServiceHandle{svc: app.IssueSvc},
			sink:   postgres.NewObservationSink(app.DB),
			approval: &approvalServiceHandle{svc: app.ApprovalSvc},
		}
		daemonWSHandler := handler.NewHandler(wsBridge, logger)
		engine.GET("/ws/daemon", wshandler.Handler(daemonWSHandler, logger))
	}

	app.httpServer = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return app, nil
}

// Serve runs the HTTP server until ctx is cancelled. Blocks.
func (a *Application) Serve(ctx context.Context) error {
	if a.httpServer == nil {
		return errors.New("app: http server not initialized")
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("app: serving HTTP",
			"addr", a.cfg.HTTPAddr,
			"db", a.DB != nil,
		)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("app: shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Close releases every resource held by the application.
func (a *Application) Close() {
	a.logger.Info("app: closing")
	if a.DB != nil {
		_ = a.DB.Close()
	}
}

// Logger returns the application logger.
func (a *Application) Logger() *slog.Logger { return a.logger }

// newLogger builds a slog logger at the configured level.
func newLogger(level string) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info", "":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q", level)
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), nil
}
// evalAutoRunner adapts *evalsvc.Service to the daemon_runtime.EvalAutoRunner
// interface (which returns just an error so the daemon can fire-and-forget).
type evalAutoRunner struct {
	svc *evalsvc.Service
}

func (e evalAutoRunner) AutoRun(ctx context.Context, workspaceID, issueID string, harnessID *string) error {
	_, err := e.svc.AutoRun(ctx, workspaceID, issueID, harnessID)
	return err
}
