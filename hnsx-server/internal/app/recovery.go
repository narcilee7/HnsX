// Package app recovery rebuilds scheduler state from Postgres on boot.
//
// The control plane keeps pending sessions in a worker.SessionQueue and live
// workers in a worker.Registry. Both are in-memory by default (Redis is
// optional). On restart we rehydrate them from the authoritative Postgres
// state so that pending/running work is not lost.
package app

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/internal/worker"
)

// RecoverSchedulingState rebuilds the worker queue and registry from Postgres.
// It is safe to call when worker services are disabled (no-op).
func (a *Application) RecoverSchedulingState(ctx context.Context, log *zap.Logger) error {
	if a.WorkerService == nil {
		return nil
	}
	if err := a.recoverPendingSessions(ctx, log); err != nil {
		return fmt.Errorf("recover pending sessions: %w", err)
	}
	if err := a.WorkerService.RecoverWorkers(ctx); err != nil {
		return fmt.Errorf("recover workers: %w", err)
	}
	return nil
}

func (a *Application) recoverPendingSessions(ctx context.Context, log *zap.Logger) error {
	_ = ctx
	// Single-tenant default. Multi-tenant recovery iterates every tenant
	// via DomainService.List; for V1.1 the default tenant is sufficient.
	tid := tenant.DefaultID

	sessions, err := a.SessionService.ListPending(tid)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}

	items := make([]*worker.SessionRequest, 0, len(sessions))
	var missingDomains int
	for _, sess := range sessions {
		domain, err := a.DomainService.Get(tid, sess.DomainID)
		if err != nil {
			missingDomains++
			log.Warn("recovery: skipping session with missing domain",
				zap.String("session_id", sess.ID),
				zap.String("domain_id", sess.DomainID),
				zap.Error(err))
			continue
		}

		req, err := sessionToRequest(sess, domain)
		if err != nil {
			log.Warn("recovery: skipping session, cannot build request",
				zap.String("session_id", sess.ID),
				zap.Error(err))
			continue
		}
		items = append(items, req)
	}

	if err := a.WorkerService.Queue().Recover(items); err != nil {
		return err
	}

	log.Info("recovery: pending sessions rehydrated",
		zap.Int("found", len(sessions)),
		zap.Int("queued", len(items)),
		zap.Int("missing_domains", missingDomains))
	return nil
}

func sessionToRequest(sess *model.Session, domain *domainmodel.RegisteredDomain) (*worker.SessionRequest, error) {
	if sess == nil {
		return nil, fmt.Errorf("nil session")
	}
	triggerJSON, err := json.Marshal(sess.Trigger)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger: %w", err)
	}

	var specJSON string
	if domain != nil && domain.Spec != nil {
		b, err := json.Marshal(domain.Spec)
		if err != nil {
			return nil, fmt.Errorf("marshal domain spec: %w", err)
		}
		specJSON = string(b)
	}

	return &worker.SessionRequest{
		SessionID:          sess.ID,
		DomainID:           sess.DomainID,
		DomainVersion:      sess.DomainVersion,
		DomainSpecJSON:     specJSON,
		TriggerPayloadJSON: string(triggerJSON),
		TraceID:            "", // not persisted on SessionRecord in V1.1
		CorrelationID:      "",
		EnqueuedAt:         sess.StartedAt,
	}, nil
}
