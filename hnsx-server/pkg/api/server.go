package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/core/domain"
	hsxcore "github.com/hnsx-io/hnsx/core/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	hsxsession "github.com/hnsx-io/hnsx/server/pkg/session"
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
// database (which may be NoDB).
type Server struct {
	BuildInfo BuildInfo
	DB        *db.DB
	Executor  *hsxsession.Executor

	mu           sync.RWMutex
	domains      map[string]*registeredDomain // keyed by domain_id
	sessions     map[string]*registeredSession
	bsessions    map[string]*hsxsession.Broadcaster
	shutdownOnce sync.Once
	httpServer   *http.Server
}

// registeredDomain wraps a parsed DomainSpec with metadata.
type registeredDomain struct {
	ID          string             `json:"id"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Spec        *domain.DomainSpec `json:"-"`
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
	Result        *hsxcore.Result `json:"result,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// NewServer constructs an API Server. The BuildInfo should be supplied by
// the main package; pass an empty struct for tests.
func NewServer(build BuildInfo, database *db.DB, executor *hsxsession.Executor) *Server {
	return &Server{
		BuildInfo: build,
		DB:        database,
		Executor:  executor,
		domains:   map[string]*registeredDomain{},
		sessions:  map[string]*registeredSession{},
		bsessions: map[string]*hsxsession.Broadcaster{},
	}
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

func (s *Server) attachBroadcaster(sessionID string) *hsxsession.Broadcaster {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bc, ok := s.bsessions[sessionID]; ok {
		return bc
	}
	bc := hsxsession.NewBroadcaster()
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

// RegisterBootstrapDomain inserts an already-validated *domain.DomainSpec
// into the in-process registry. Intended for the `bootstrapDomains`
// path in main, not for the public API.
//
// Public callers should use the POST /api/v1/domains handler instead.
func (s *Server) RegisterBootstrapDomain(spec any) {
	ds, ok := spec.(*domain.DomainSpec)
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
