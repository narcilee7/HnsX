// Package service implements the Store application use cases.
package service

import (
	"context"
	"fmt"

	"github.com/hnsx-io/hnsx/server/internal/store"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Service builds and caches store backends per domain.
type Service struct {
	db *db.DB
}

// NewService constructs a Store service backed by the supplied DB sentinel.
func NewService(db *db.DB) *Service {
	return &Service{db: db}
}

// BackendFor returns a store Backend for the supplied DomainSpec store block.
//
// When the server has a Postgres connection and the domain requests the
// "postgres" backend, a PostgresBackend is returned; otherwise it falls back to
// an in-memory backend.
func (s *Service) BackendFor(ctx context.Context, cfg *spec.StoreConfig) (store.Backend, error) {
	// Default to in-memory when no DB is configured.
	if s.db == nil || s.db.IsNoDB() {
		return store.NewBackendFromSpec(cfg)
	}

	// If any namespace asks for postgres, use the Postgres backend for the
	// whole domain. Mixed backends within one domain are left for future work.
	requested := requestedBackend(cfg)
	if requested == "postgres" {
		pb := store.NewPostgresBackend(s.db.GormDB)
		if err := pb.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("store: migrate postgres backend: %w", err)
		}
		return pb, nil
	}

	return store.NewBackendFromSpec(cfg)
}

func requestedBackend(cfg *spec.StoreConfig) string {
	if cfg == nil {
		return ""
	}
	for _, nsCfg := range []spec.StoreNamespaceConfig{cfg.Context, cfg.Knowledge, cfg.Ephemeral} {
		if nsCfg.Backend == "postgres" {
			return "postgres"
		}
	}
	return ""
}
