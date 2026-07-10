package service

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/session/repository"
)

func TestService_CreateAndGet(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	sess, err := svc.Create(CreateParams{
		SessionID:     "s-1",
		DomainID:      "d-1",
		DomainVersion: "1.0.0",
		Orchestration: "single",
		Trigger:       map[string]any{"q": "hello"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sess.State != model.StatePending {
		t.Fatalf("state = %q", sess.State)
	}
	if sess.StartedAt.IsZero() {
		t.Fatal("missing started_at")
	}

	got, err := svc.Get("s-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DomainID != "d-1" {
		t.Fatalf("domain_id = %q", got.DomainID)
	}
}

func TestService_CreateInvalid(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	if _, err := svc.Create(CreateParams{SessionID: "", DomainID: "d"}); err != model.ErrInvalidSession {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}
	if _, err := svc.Create(CreateParams{SessionID: "s", DomainID: ""}); err != model.ErrInvalidSession {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}
}

func TestService_MarkRunning(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	sess, _ := svc.Create(CreateParams{SessionID: "s", DomainID: "d", Orchestration: "single"})
	if sess.State != model.StatePending {
		t.Fatalf("initial state = %q", sess.State)
	}

	sess, err := svc.MarkRunning("s")
	if err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if sess.State != model.StateRunning {
		t.Fatalf("state = %q", sess.State)
	}

	// Second call should fail: not pending anymore.
	if _, err := svc.MarkRunning("s"); err != model.ErrInvalidSession {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}
}

func TestService_MarkCompleted(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	_, _ = svc.Create(CreateParams{SessionID: "s", DomainID: "d", Orchestration: "single"})
	result := &runtime.Result{Mode: "single"}
	sess, err := svc.MarkCompleted("s", result)
	if err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if sess.State != model.StateCompleted {
		t.Fatalf("state = %q", sess.State)
	}
	if sess.CompletedAt == nil {
		t.Fatal("missing completed_at")
	}
	if sess.Result != result {
		t.Fatal("result not stored")
	}
}

func TestService_Cancel(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	_, _ = svc.Create(CreateParams{SessionID: "s", DomainID: "d", Orchestration: "single"})
	sess, err := svc.Cancel("s")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if sess.State != model.StateCancelled {
		t.Fatalf("state = %q", sess.State)
	}

	// Cancel again should fail.
	if _, err := svc.Cancel("s"); err != model.ErrAlreadyTerminal {
		t.Fatalf("expected ErrAlreadyTerminal, got %v", err)
	}
}

func TestService_Rerun(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	original, _ := svc.Create(CreateParams{
		SessionID:     "s",
		DomainID:      "d",
		DomainVersion: "1.0.0",
		Orchestration: "single",
		Trigger:       map[string]any{"q": "hello"},
	})

	newSess, err := svc.Rerun(original)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if newSess.ID == original.ID {
		t.Fatal("rerun should create a new session id")
	}
	if newSess.DomainID != "d" || newSess.DomainVersion != "1.0.0" {
		t.Fatalf("rerun domain metadata mismatch")
	}
	if newSess.Trigger["q"] != "hello" {
		t.Fatal("rerun trigger not preserved")
	}
}

func TestSession_IsTerminal(t *testing.T) {
	terminal := []model.State{model.StateCompleted, model.StateFailed, model.StateCancelled}
	for _, s := range terminal {
		if !(&model.Session{State: s}).IsTerminal() {
			t.Fatalf("state %q should be terminal", s)
		}
	}
	nonTerminal := []model.State{model.StatePending, model.StateRunning, model.StatePaused}
	for _, s := range nonTerminal {
		if (&model.Session{State: s}).IsTerminal() {
			t.Fatalf("state %q should not be terminal", s)
		}
	}
}

func TestSession_Duration(t *testing.T) {
	start := time.Now().UTC().Add(-time.Second)
	s := &model.Session{StartedAt: start, State: model.StateRunning}
	if d := s.Duration(); d < time.Millisecond {
		t.Fatalf("duration too small: %v", d)
	}

	completed := start.Add(500 * time.Millisecond)
	s.CompletedAt = &completed
	if d := s.Duration(); d != 500*time.Millisecond {
		t.Fatalf("duration = %v", d)
	}
}

func TestRepository_ByDomain(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	for _, id := range []string{"s1", "s2", "s3"} {
		domainID := "d1"
		if id == "s3" {
			domainID = "d2"
		}
		_ = repo.Save(&model.Session{ID: id, DomainID: domainID, State: model.StatePending})
	}

	list, err := repo.ByDomain("d1")
	if err != nil {
		t.Fatalf("by domain: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d", len(list))
	}
}
