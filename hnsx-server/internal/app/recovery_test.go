package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	domainrepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	sessionrepo "github.com/hnsx-io/hnsx/server/internal/session/repository"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	workerrepo "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func TestRecoverSchedulingState_PendingSessions(t *testing.T) {
	domainRepo := domainrepo.NewInMemoryRepository()
	sessionRepo := sessionrepo.NewInMemoryRepository()

	domainSvc := domainservice.NewService(domainRepo)
	sessionSvc := sessionservice.NewService(sessionRepo)
	workerSvc := workerservice.NewService(workerrepo.NewInMemoryRepository())

	app := &Application{
		DomainService:  domainSvc,
		SessionService: sessionSvc,
		WorkerService:  workerSvc,
		Log:            zap.NewNop(),
	}

	tid := tenant.DefaultID

	// Seed domain.
	ds := &domain.DomainSpec{
		ID:      "recovery-domain",
		Version: "1.0.0",
		Harness: domain.HarnessSpec{
			Agents: map[string]domain.AgentSpec{
				"agent": {ID: "agent", Provider: "noop", Adapter: domain.AdapterConfig{Kind: "noop"}},
			},
			Session: domain.SessionSpec{Mode: domain.Single, Agent: "agent"},
		},
	}
	_, err := domainSvc.Register(tid, ds)
	require.NoError(t, err)

	// Seed pending session.
	now := time.Now().UTC()
	sess := &model.Session{
		ID:            "s-pending",
		DomainID:      ds.ID,
		DomainVersion: ds.Version,
		State:         model.StatePending,
		Trigger:       map[string]any{"q": "hello"},
		StartedAt:     now,
	}
	require.NoError(t, sessionRepo.Save(tid, sess))

	// Recovery should load the pending session into the queue.
	require.NoError(t, app.RecoverSchedulingState(context.Background(), zap.NewNop()))

	if workerSvc.QueueLen() != 1 {
		t.Fatalf("queue len = %d, want 1", workerSvc.QueueLen())
	}

	req, ok := workerSvc.Queue().Dequeue(context.Background(), nil)
	require.True(t, ok)
	require.Equal(t, "s-pending", req.SessionID)
	require.Equal(t, ds.ID, req.DomainID)
	require.Contains(t, req.DomainSpecJSON, `"id":"recovery-domain"`)
	require.Contains(t, req.TriggerPayloadJSON, `"q":"hello"`)
}

func TestSessionToRequest(t *testing.T) {
	sess := &model.Session{
		ID:            "s-1",
		DomainID:      "d-1",
		DomainVersion: "v1",
		State:         model.StatePending,
		Trigger:       map[string]any{"q": "hi"},
		StartedAt:     time.Now().UTC(),
	}
	d := &domainmodel.RegisteredDomain{
		ID:      "d-1",
		Version: "v1",
		Spec: &domain.DomainSpec{
			ID: "d-1",
			Harness: domain.HarnessSpec{
				Agents: map[string]domain.AgentSpec{
					"a": {ID: "a"},
				},
			},
		},
	}

	req, err := sessionToRequest(sess, d)
	require.NoError(t, err)
	require.Equal(t, "s-1", req.SessionID)
	require.Equal(t, "d-1", req.DomainID)
	require.Equal(t, "v1", req.DomainVersion)
	require.Contains(t, req.DomainSpecJSON, "d-1")
	require.Contains(t, req.TriggerPayloadJSON, "hi")
}

func TestSessionToRequest_NilSession(t *testing.T) {
	_, err := sessionToRequest(nil, nil)
	require.Error(t, err)
}
