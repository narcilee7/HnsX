package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	auditservice "github.com/hnsx-io/hnsx/server/internal/audit/service"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
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
	// App is the composed server-side application. Existing transitional
	// fields below are populated from App in the constructor so that Phase 0
	// handlers continue to work unchanged; Phase 1 will remove them.
	App *app.Application

	BuildInfo BuildInfo
	DB        *db.DB
	Executor  *pkgexecutor.Executor
	AppState  *app.State

	// V1.1 worker pool. May be nil when the server is started without the
	// gRPC control plane (legacy local-executor mode).
	WorkerRegistry *worker.Registry
	SessionQueue   worker.SessionQueue

	// PolicyService loads domain policy into the policy repository.
	PolicyService *policyservice.Service

	// AuditService records and queries immutable audit entries.
	AuditService *auditservice.Service

	// TraceService records and queries observation traces.
	TraceService *traceservice.Service

	// EvalService manages eval sets and runs.
	EvalService *evalservice.Service

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
// BuildInfo should be supplied by the main package; pass an empty struct for tests.
func NewServer(build BuildInfo, application *app.Application) *Server {
	return NewServerWithWorkerPool(build, application)
}

// NewServerWithWorkerPool constructs an API Server wired to the V1.1 worker
// pool through the supplied Application. When Application.WorkerRegistry and
// SessionQueue are non-nil, session triggers are enqueued for Python workers
// instead of executed locally.
func NewServerWithWorkerPool(build BuildInfo, application *app.Application) *Server {
	return &Server{
		App:            application,
		BuildInfo:      build,
		DB:             application.DB,
		Executor:       application.Executor,
		AppState:       application.State,
		WorkerRegistry: application.WorkerRegistry,
		SessionQueue:   application.SessionQueue,
		PolicyService:  application.PolicyService,
		AuditService:   application.AuditService,
		TraceService:   application.TraceService,
		EvalService:    application.EvalService,
	}
}

// WithWorkerPool wires an existing server into the V1.1 worker pool. Used by
// tests and by main when the gRPC control plane is enabled.
func (s *Server) WithWorkerPool(reg *worker.Registry, q worker.SessionQueue) *Server {
	s.WorkerRegistry = reg
	s.SessionQueue = q
	return s
}

// LoadDomainPolicy persists the policy for the named domain. It is called
// automatically after domain registration/update and bootstrap seeding.
func (s *Server) LoadDomainPolicy(ctx context.Context, domainID string) error {
	if s.PolicyService == nil {
		return nil
	}
	_, d, ok := queries.GetDomain(s.AppState, tenant.FromContext(ctx), domainID)
	if !ok {
		return ErrDomainNotFound
	}
	return s.PolicyService.LoadDomainPolicy(domainID, d.Spec)
}

// Handler returns the http.Handler with the entire API surface mounted.
func (s *Server) Handler() http.Handler {
	return s.drainMiddleware(newRouter(s))
}

// Listen starts the HTTP server on addr. Blocks until Shutdown is called or
// the listener fails.
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
// New requests receive 503 Service Unavailable. Call before Shutdown.
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

// drainMiddleware tracks in-flight requests and rejects new ones once draining.
func (s *Server) drainMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.draining.Load() {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.activeRequests.Add(1)
		defer s.activeRequests.Done()
		next.ServeHTTP(w, r)
	})
}

// timeoutCtx derives a request-scoped context with the configured timeout.
func (s *Server) timeoutCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 30*time.Second)
}

// PublishObservation forwards an observation into the named session's
// broadcaster so SSE clients see it. It is the bridge between the gRPC
// worker StreamChannel and the HTTP /events endpoint.
func (s *Server) PublishObservation(sessionID string, obs runtime.Observation) bool {
	if s.AppState == nil {
		return false
	}
	return s.AppState.PublishObservation(sessionID, obs)
}

// UpdateSessionState updates the in-memory session state. Called by the
// scheduler when the worker reports a terminal status update.
func (s *Server) UpdateSessionState(tenantID tenant.ID, sessionID, state string) {
	if s.AppState == nil {
		return
	}
	s.AppState.UpdateSessionState(tenantID, sessionID, state)
}

// RegisterBootstrapDomain inserts an already-validated *spec.DomainSpec
// into the in-process registry. Intended for the `seed-from` path in main,
// not for the public API.
func (s *Server) RegisterBootstrapDomain(tenantID tenant.ID, v any) {
	if s.AppState == nil {
		return
	}
	ds, ok := v.(*spec.DomainSpec)
	if !ok {
		return
	}
	now := time.Now().UTC()
	s.AppState.RegisterDomain(tenantID, &app.RegisteredDomain{
		ID:          ds.ID,
		Version:     ds.Version,
		Description: ds.Description,
		Spec:        ds,
		Harness:     ds.Harness,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_ = s.LoadDomainPolicy(tenant.NewContext(context.Background(), tenantID), ds.ID)
}
