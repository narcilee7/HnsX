package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
)

// BuildInfo describes this build of hnsx-server. Set by main at process start.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Built     string `json:"built"`
	GoVersion string `json:"go_version"`
}

// Server is the API layer. It owns the in-process domain registry, the
// session registry, the live Session broadcasters, and a reference to the
// database (which may be NoDB). In V1.1 it also holds the worker registry and
// session queue so REST session creation can enqueue work for Python workers.
type Server struct {
	BuildInfo BuildInfo
	DB        *db.DB
	Executor  *pkgexecutor.Executor

	// V1.1 worker pool. May be nil when the server is started without the
	// gRPC control plane (legacy local-executor mode).
	WorkerRegistry *worker.Registry
	SessionQueue   *worker.SessionQueue

	mu           sync.RWMutex
	domains      map[string]*registeredDomain // keyed by domain_id
	sessions     map[string]*registeredSession
	bsessions    map[string]*broadcaster.Broadcaster
	shutdownOnce sync.Once
	httpServer   *http.Server
}

// registeredDomain wraps a parsed DomainSpec with metadata.
type registeredDomain struct {
	ID          string             `json:"id"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Spec        *spec.DomainSpec `json:"-"`
	Harness     any                `json:"harness,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// Domain is a re-export alias used by Sessions results so callers can avoid
// importing pkg/core/domain everywhere.
type Domain = registeredDomain

// registeredSession is the runtime metadata for one Session run.
type registeredSession struct {
	ID            string          `json:"id"`
	DomainID      string          `json:"domain_id"`
	DomainVersion string          `json:"domain_version"`
	Orchestration string          `json:"orchestration"`
	State         string          `json:"state"`
	Trigger       map[string]any  `json:"trigger,omitempty"`
	Result        *runtime.Result `json:"result,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// NewServer constructs an API Server. The BuildInfo should be supplied by
// the main package; pass an empty struct for tests.
func NewServer(build BuildInfo, database *db.DB, executor *pkgexecutor.Executor) *Server {
	return NewServerWithWorkerPool(build, database, executor, nil, nil)
}

// NewServerWithWorkerPool constructs an API Server wired to the V1.1 worker
// pool. When WorkerRegistry and SessionQueue are non-nil, session triggers
// are enqueued for Python workers instead of executed locally.
func NewServerWithWorkerPool(build BuildInfo, database *db.DB, executor *pkgexecutor.Executor, reg *worker.Registry, q *worker.SessionQueue) *Server {
	return &Server{
		BuildInfo:      build,
		DB:             database,
		Executor:       executor,
		WorkerRegistry: reg,
		SessionQueue:   q,
		domains:        map[string]*registeredDomain{},
		sessions:       map[string]*registeredSession{},
		bsessions:      map[string]*broadcaster.Broadcaster{},
	}
}

// WithWorkerPool wires an existing server into the V1.1 worker pool. Used by
// tests and by main when the gRPC control plane is enabled.
func (s *Server) WithWorkerPool(reg *worker.Registry, q *worker.SessionQueue) *Server {
	s.WorkerRegistry = reg
	s.SessionQueue = q
	return s
}

// Handler returns the http.Handler with the entire API surface mounted.
func (s *Server) Handler() http.Handler {
	return newRouter(s)
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

// timeoutCtx derives a request-scoped context with the configured timeout.
func (s *Server) timeoutCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 30*time.Second)
}

// ----------------------------------------------------------------------------
// helpers used by handlers
// ----------------------------------------------------------------------------

func (s *Server) registerDomain(d *registeredDomain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains[d.ID] = d
}

func (s *Server) lookupDomain(id string) (*registeredDomain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domains[id]
	return d, ok
}

func (s *Server) listDomainItems() []*registeredDomain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*registeredDomain, 0, len(s.domains))
	for _, d := range s.domains {
		out = append(out, d)
	}
	return out
}

func (s *Server) registerSession(sess *registeredSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

func (s *Server) lookupSession(id string) (*registeredSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Server) attachBroadcaster(sessionID string) *broadcaster.Broadcaster {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bc, ok := s.bsessions[sessionID]; ok {
		return bc
	}
	bc := broadcaster.NewBroadcaster()
	s.bsessions[sessionID] = bc
	return bc
}

func (s *Server) detachBroadcaster(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bc, ok := s.bsessions[sessionID]; ok {
		bc.Close()
		delete(s.bsessions, sessionID)
	}
}

func (s *Server) listSessionItems() []*registeredSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*registeredSession, 0, len(s.sessions))
	for _, s := range s.sessions {
		out = append(out, s)
	}
	return out
}

// PublishObservation forwards an observation into the named session's
// broadcaster so SSE clients see it. It is the bridge between the gRPC
// worker StreamChannel and the HTTP /events endpoint. Returns false if the
// session has no broadcaster (e.g. it was triggered before V1.1 or has
// already been cleaned up).
func (s *Server) PublishObservation(sessionID string, obs runtime.Observation) bool {
	bc := s.attachBroadcaster(sessionID)
	ctx := context.Background()
	if err := bc.Publish(ctx, obs); err != nil {
		return false
	}
	return true
}

// UpdateSessionState updates the in-memory session state. Called by the
// scheduler when the worker reports a terminal status update.
func (s *Server) UpdateSessionState(sessionID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	sess.State = state
	if state == "completed" || state == "failed" || state == "cancelled" || state == "canceled" {
		now := time.Now().UTC()
		sess.CompletedAt = &now
	}
}

// RegisterBootstrapDomain inserts an already-validated *domain.DomainSpec
// into the in-process registry. Intended for the `bootstrapDomains`
// path in main, not for the public API.
//
// Public callers should use the POST /api/v1/domains handler instead.
func (s *Server) RegisterBootstrapDomain(v any) {
	ds, ok := v.(*spec.DomainSpec)
	if !ok {
		return
	}
	now := time.Now().UTC()
	d := &registeredDomain{
		ID:          ds.ID,
		Version:     ds.Version,
		Description: ds.Description,
		Spec:        ds,
		Harness:     ds.Harness,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.registerDomain(d)
}
