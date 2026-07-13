package store

import (
	"fmt"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// NewBackendFromSpec builds a Backend from a DomainSpec store declaration.
//
// Missing namespaces default to in_memory. Unsupported backends return an
// error so domain validation can surface the problem early.
func NewBackendFromSpec(cfg *domain.StoreConfig) (Backend, error) {
	if cfg == nil {
		return NewInMemoryBackend(), nil
	}
	for ns, nsCfg := range map[Namespace]domain.StoreNamespaceConfig{
		NamespaceContext:   cfg.Context,
		NamespaceKnowledge: cfg.Knowledge,
		NamespaceEphemeral: cfg.Ephemeral,
	} {
		backend := nsCfg.Backend
		if backend == "" {
			backend = "in_memory"
		}
		if backend != "in_memory" {
			return nil, fmt.Errorf("store namespace %q backend %q is not supported in this build", ns, backend)
		}
	}
	return NewInMemoryBackend(), nil
}
