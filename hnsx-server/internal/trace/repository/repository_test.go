package repository

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

func TestInMemoryRepository_SaveAndQuery(t *testing.T) {
	r := NewInMemoryRepository()

	rec1 := &model.ObservationRecord{
		TraceID:   "t1",
		SessionID: "s1",
		DomainID:  "d1",
		Kind:      "agent_invoke",
		CreatedAt: time.Now().UTC(),
	}
	rec2 := &model.ObservationRecord{
		TraceID:   "t1",
		SessionID: "s1",
		DomainID:  "d1",
		Kind:      "agent_text",
		CreatedAt: time.Now().UTC().Add(time.Second),
	}
	rec3 := &model.ObservationRecord{
		TraceID:   "t2",
		SessionID: "s2",
		DomainID:  "d2",
		Kind:      "session_start",
		CreatedAt: time.Now().UTC(),
	}

	for _, rec := range []*model.ObservationRecord{rec1, rec2, rec3} {
		if err := r.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	bySession, err := r.BySession("s1")
	if err != nil {
		t.Fatalf("by session: %v", err)
	}
	if len(bySession) != 2 {
		t.Fatalf("expected 2 records for s1, got %d", len(bySession))
	}
	if bySession[0].Kind != "agent_invoke" {
		t.Fatalf("expected chronological order, first=%s", bySession[0].Kind)
	}

	byTrace, err := r.ByTrace("t1")
	if err != nil {
		t.Fatalf("by trace: %v", err)
	}
	if len(byTrace) != 2 {
		t.Fatalf("expected 2 records for t1, got %d", len(byTrace))
	}
}

func TestInMemoryRepository_FromRuntime(t *testing.T) {
	obs := runtime.Observation{
		Kind:      "agent_text",
		SessionID: "s1",
		DomainID:  "d1",
		AgentID:   "a1",
		Cost: &runtime.Cost{
			PromptTokens:     10,
			CompletionTokens: 20,
			CostUSD:          0.001,
			LatencyMs:        100,
		},
		Timestamp: time.Now().UTC(),
	}
	rec := model.FromRuntime(obs)
	if rec.SessionID != "s1" {
		t.Fatalf("expected session s1, got %s", rec.SessionID)
	}
	if rec.PromptTokens != 10 || rec.CompletionTokens != 20 {
		t.Fatal("cost not copied")
	}
}

func TestInMemoryRepository_SaveNil(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(nil); err == nil {
		t.Fatal("expected error for nil record")
	}
}
