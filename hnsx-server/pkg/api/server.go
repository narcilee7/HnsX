package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	approvalservice "github.com/hnsx-io/hnsx/server/internal/approval/service"
	auditservice "github.com/hnsx-io/hnsx/server/internal/audit/service"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	secretservice "github.com/hnsx-io/hnsx/server/internal/secret/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// BuildInfo describes this build of hnsx-server. Set by main at process start.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Built     string `json:"built"`
	GoVersion string `json:"go_version"`
}

// Server is the API layer. It delegates all business state to the app layer
// (internal/app) and only handles HTTP protocol concerns: routing, decoding,
// encoding, and SSE streaming.
type Server struct {
	// App is the composed server-side application.
	App *app.Application

	BuildInfo BuildInfo
	DB        *db.DB
	Executor  *pkgexecutor.Executor
	AppState  *app.State

	// PolicyService loads domain policy into the policy repository.
	PolicyService *policyservice.Service

	// AuditService records and queries immutable audit entries.
	AuditService *auditservice.Service

	// TraceService records and queries observation traces.
	TraceService *traceservice.Service

	// EvalService manages eval sets and runs.
	EvalService *evalservice.Service

	// ApprovalService implements the human-in-the-loop gate; nil is OK
	// only if the operator never wires any domain with require_human_approval.
	ApprovalService *approvalservice.Service

	// SecretService resolves ${secret:name} placeholders and persists
	// AES-GCM encrypted values; nil means the operator did not configure
	// HNSX_SECRET_KEY and the control plane refused to start.
	SecretService *secretservice.Service

	// WorkerService manages worker registration, scheduling, and session queueing.
	WorkerService *workerservice.Service

	// DomainCommands exposes domain lifecycle use cases.
	DomainCommands *commands.DomainCommands

	// SessionCommands exposes session lifecycle use cases.
	SessionCommands *commands.SessionCommands

	// Queries exposes read-only application queries.
	Queries *queries.Queries

	// ConnectHandler serves the Connect-RPC control plane on /hnsx.v1.* paths.
	// When nil the HTTP server exposes only the REST API.
	ConnectHandler http.Handler

	// TemplatesIndexPath is the path to the template market index YAML.
	// When empty the template gallery endpoint returns an empty list.
	TemplatesIndexPath string

	shutdownOnce   sync.Once
	httpServer     *http.Server
	activeRequests sync.WaitGroup
	draining       atomic.Bool
}

// ErrDomainNotFound is returned when a requested domain is not registered.
var ErrDomainNotFound = errors.New("domain not found")

// Domain is a re-export alias used by Sessions results so callers can avoid
// importing internal/app everywhere.
type Domain = app.RegisteredDomain

// NewServer constructs an API Server wired to the supplied Application.
// The worker pool is wired automatically when the Application has one.
func NewServer(build BuildInfo, application *app.Application) *Server {
	return &Server{
		App:                application,
		BuildInfo:          build,
		DB:                 application.DB,
		Executor:           application.Executor,
		AppState:           application.State,
		PolicyService:      application.PolicyService,
		AuditService:       application.AuditService,
		TraceService:       application.TraceService,
		EvalService:        application.EvalService,
		ApprovalService:    application.ApprovalService,
		SecretService:      application.SecretService,
		WorkerService:      application.WorkerService,
		DomainCommands:     commands.NewDomainCommands(application.DomainService),
		SessionCommands:    commands.NewSessionCommands(application.SessionService, application.DomainService, application.WorkerService, application.Executor, application.State),
		Queries:            queries.NewQueries(application.DomainService, application.SessionService),
		TemplatesIndexPath: "templates/index.yaml",
	}
}

// WithConnectHandler attaches the Connect-RPC handler mux to the HTTP server.
// It returns the same *Server for chaining.
func (s *Server) WithConnectHandler(h http.Handler) *Server {
	s.ConnectHandler = h
	return s
}

// LoadDomainPolicy persists the policy for the named domain.
func (s *Server) LoadDomainPolicy(ctx context.Context, domainID string) error {
	if s.PolicyService == nil {
		return nil
	}
	_, d, ok := s.Queries.GetDomain(tenant.FromContext(ctx), domainID)
	if !ok {
		return ErrDomainNotFound
	}
	return s.PolicyService.LoadDomainPolicy(domainID, d.Spec)
}

// Handler returns the gin.Engine with the entire API surface mounted.
func (s *Server) Handler() *gin.Engine {
	return newRouter(s)
}

// Listen starts the HTTP server on addr. Blocks until Shutdown is called.
func (s *Server) Listen(addr string) error {
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown stops the HTTP server gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.shutdownOnce.Do(func() {
		if s.httpServer != nil {
			err = s.httpServer.Shutdown(ctx)
		}
	})
	return err
}

// Drain marks the server as draining and waits for active requests to finish.
func (s *Server) Drain(ctx context.Context) error {
	s.draining.Store(true)
	done := make(chan struct{})
	go func() {
		s.activeRequests.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDraining reports whether the server is currently draining.
func (s *Server) IsDraining() bool { return s.draining.Load() }

// TrackRequest marks a request as in-flight. Callers must call Done().
func (s *Server) TrackRequest() { s.activeRequests.Add(1) }

// DoneRequest marks a request as finished.
func (s *Server) DoneRequest() { s.activeRequests.Done() }

// timeoutCtx derives a request-scoped context with the configured timeout.
func (s *Server) timeoutCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 30*time.Second)
}

// PublishObservation forwards an observation into the named session's
// broadcaster so SSE clients see it.
func (s *Server) PublishObservation(sessionID string, obs runtime.Observation) bool {
	if s.AppState == nil {
		return false
	}
	return s.AppState.PublishObservation(sessionID, obs)
}

// RegisterBootstrapDomain inserts an already-validated *spec.DomainSpec
// into the domain registry. Intended for the `seed-from` path in main.
func (s *Server) RegisterBootstrapDomain(tenantID tenant.ID, v *spec.DomainSpec) {
	if s.App == nil || s.App.DomainService == nil {
		return
	}
	if v == nil {
		return
	}
	if _, err := s.App.DomainService.Register(tenantID, v); err != nil {
		return
	}
	_ = s.LoadDomainPolicy(tenant.NewContext(context.Background(), tenantID), v.ID)
}
