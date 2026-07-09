package repository

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	domainrepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	internalsession "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/testutil"
)

func TestPostgresSessionRepository_SaveAndGet(t *testing.T) {
	database := testutil.OpenTestDB(t)
	defer database.Close()

	// Ensure a domain exists so the FK constraint is satisfied.
	domainRepo := domainrepo.NewPostgresRepository(database)
	_ = domainRepo.Delete("session-test-domain")
	spec := &spec.DomainSpec{
		ID:      "session-test-domain",
		Version: "1.0.0",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"agent": {ID: "agent", Provider: "noop", Adapter: spec.AdapterConfig{Kind: "noop"}},
			},
			Session: spec.SessionSpec{Mode: "single", Agent: "agent"},
		},
	}
	if err := domainRepo.Save(&model.RegisteredDomain{
		ID:      spec.ID,
		Version: spec.Version,
		Spec:    spec,
	}); err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	repo := NewPostgresRepository(database)
	_ = repo.Delete("s-test-1")

	sess := &internalsession.Session{
		ID:            "s-test-1",
		DomainID:      "session-test-domain",
		DomainVersion: "1.0.0",
		Orchestration: "single",
		State:         internalsession.StatePending,
		Trigger:       map[string]any{"q": "hello"},
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.Save(sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.ByID("s-test-1")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.DomainID != "session-test-domain" {
		t.Fatalf("domain_id = %q", got.DomainID)
	}
	if got.State != internalsession.StatePending {
		t.Fatalf("state = %q", got.State)
	}
	if got.Trigger["q"] != "hello" {
		t.Fatal("trigger not round-tripped")
	}

	// Update to completed with result.
	sess.State = internalsession.StateCompleted
	sess.Result = &runtime.Result{Mode: "single"}
	completed := time.Now().UTC()
	sess.CompletedAt = &completed
	if err := repo.Save(sess); err != nil {
		t.Fatalf("save update: %v", err)
	}

	got, err = repo.ByID("s-test-1")
	if err != nil {
		t.Fatalf("by id after update: %v", err)
	}
	if got.State != internalsession.StateCompleted {
		t.Fatalf("state after update = %q", got.State)
	}
	if got.Result == nil || got.Result.Mode != "single" {
		t.Fatal("result not round-tripped")
	}

	byDomain, err := repo.ByDomain("session-test-domain")
	if err != nil {
		t.Fatalf("by domain: %v", err)
	}
	if len(byDomain) != 1 {
		t.Fatalf("by domain len = %d", len(byDomain))
	}

	list, err := repo.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	found := false
	for _, s := range list {
		if s.ID == "s-test-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("session not in list")
	}

	_ = repo.Delete("s-test-1")
	_ = domainRepo.Delete("session-test-domain")
}
