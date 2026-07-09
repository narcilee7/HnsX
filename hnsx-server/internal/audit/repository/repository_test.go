package repository

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
)

func TestInMemoryRepository_SaveAndList(t *testing.T) {
	r := NewInMemoryRepository()

	e1 := &model.Entry{
		SessionID: "s1",
		DomainID:  "d1",
		Action:    "tool_call",
		Actor:     "agent-a",
		ActorType: model.ActorTypeAgent,
		Decision:  model.DecisionAllow,
		Timestamp: time.Now().UTC(),
	}
	e2 := &model.Entry{
		SessionID: "s2",
		DomainID:  "d1",
		Action:    "policy_decision",
		Actor:     "policy_engine",
		ActorType: model.ActorTypeSystem,
		Decision:  model.DecisionDeny,
		Timestamp: time.Now().UTC().Add(time.Second),
	}

	if err := r.Save(e1); err != nil {
		t.Fatalf("save e1: %v", err)
	}
	if err := r.Save(e2); err != nil {
		t.Fatalf("save e2: %v", err)
	}

	entries, total, err := r.List(10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Newest first.
	if entries[0].Action != "policy_decision" {
		t.Fatalf("expected newest entry first, got %s", entries[0].Action)
	}

	bySession, err := r.BySession("s1")
	if err != nil {
		t.Fatalf("by session: %v", err)
	}
	if len(bySession) != 1 {
		t.Fatalf("expected 1 entry for s1, got %d", len(bySession))
	}

	byDomain, err := r.ByDomain("d1")
	if err != nil {
		t.Fatalf("by domain: %v", err)
	}
	if len(byDomain) != 2 {
		t.Fatalf("expected 2 entries for d1, got %d", len(byDomain))
	}
}

func TestInMemoryRepository_SaveInvalid(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(&model.Entry{}); err == nil {
		t.Fatal("expected error for empty entry")
	}
	if err := r.Save(nil); err == nil {
		t.Fatal("expected error for nil entry")
	}
}
