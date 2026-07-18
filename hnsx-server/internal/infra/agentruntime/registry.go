package agentruntime

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// Registry is the concrete implementation of domain.agentruntime.Registry.
// Backends are registered at startup (in app.New) and resolved by name
// at run time. The registry is read-mostly after construction so a
// sync.RWMutex is sufficient.
type Registry struct {
	mu       sync.RWMutex
	backends map[string]agentruntime.Backend
	logger   *slog.Logger
}

// NewRegistry returns an empty registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		backends: make(map[string]agentruntime.Backend),
		logger:   logger,
	}
}

// Register adds a backend. Re-registering the same name replaces; the
// caller is responsible for ensuring the old backend is no longer in use.
func (r *Registry) Register(b agentruntime.Backend) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[b.Name()] = b
	r.logger.Info("agentruntime: registered backend", "name", b.Name())
}

// Get returns the backend with the given name, or agentruntime.ErrBackendNotFound.
func (r *Registry) Get(name string) (agentruntime.Backend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	if !ok {
		return nil, agentruntime.ErrBackendNotFound
	}
	return b, nil
}

// List returns the sorted names of all registered backends.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.backends))
	for name := range r.backends {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// _ guards against accidental signature drift on the Registry port.
var _ agentruntime.Registry = (*Registry)(nil)