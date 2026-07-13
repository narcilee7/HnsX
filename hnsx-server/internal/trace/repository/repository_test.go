package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
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
	obs := domain.Observation{
		Kind:      "agent_text",
		SessionID: "s1",
		DomainID:  "d1",
		AgentID:   "a1",
		Cost: &domain.Cost{
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

func TestInMemoryRepository_AggregateBySession(t *testing.T) {
	r := NewInMemoryRepository()
	recs := []*model.ObservationRecord{
		{SessionID: "s1", Kind: "agent_invoke", CostUSD: 0.01, PromptTokens: 5, CompletionTokens: 7, CreatedAt: time.Now().UTC()},
		{SessionID: "s1", Kind: "tool_call", CostUSD: 0.02, CreatedAt: time.Now().UTC()},
		{SessionID: "s2", Kind: "agent_invoke", CostUSD: 0.05, CreatedAt: time.Now().UTC()},
	}
	for _, rec := range recs {
		if err := r.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	byID, err := r.AggregateBySession([]string{"s1", "s2"})
	if err != nil {
		t.Fatalf("aggregate by session: %v", err)
	}
	if len(byID) != 2 {
		t.Fatalf("expected 2 session rollups, got %d", len(byID))
	}
	s1 := byID["s1"]
	if s1.AgentInvocations != 1 || s1.ToolInvocations != 1 {
		t.Fatalf("s1 invocations agent=%d tool=%d", s1.AgentInvocations, s1.ToolInvocations)
	}
	if s1.TotalCostUSD != 0.03 || s1.TotalPromptTokens != 5 || s1.TotalCompletionTokens != 7 {
		t.Fatalf("s1 rollup wrong: %+v", s1)
	}
	if byID["s2"].TotalCostUSD != 0.05 {
		t.Fatalf("s2 cost wrong: %+v", byID["s2"])
	}

	// Filtering to a single session excludes the other.
	only, err := r.AggregateBySession([]string{"s1"})
	if err != nil {
		t.Fatalf("aggregate s1: %v", err)
	}
	if _, ok := only["s2"]; ok {
		t.Fatal("s2 should be excluded when filtering to s1")
	}
}

func TestInMemoryRepository_ListSummaries_GroupsByTraceID(t *testing.T) {
	r := NewInMemoryRepository()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	recs := []*model.ObservationRecord{
		{TraceID: "trace-A", SessionID: "s1", DomainID: "d1", DomainVersion: "1", Kind: "agent_invoke", CostUSD: 0.01, PromptTokens: 5, CompletionTokens: 7, CreatedAt: t0},
		{TraceID: "trace-A", SessionID: "s1", DomainID: "d1", DomainVersion: "1", Kind: "tool_call", CostUSD: 0.02, CreatedAt: t0.Add(1 * time.Second)},
		{TraceID: "trace-A", SessionID: "s1", DomainID: "d1", DomainVersion: "1", Kind: "session_end", CreatedAt: t0.Add(2 * time.Second)},
		{TraceID: "trace-B", SessionID: "s2", DomainID: "d1", DomainVersion: "1", Kind: "agent_invoke", CostUSD: 0.05, CreatedAt: t0.Add(3 * time.Second)},
		// No trace_id: must be ignored by ListSummaries so unrelated orphan
		// rows never leak into the trace index.
		{TraceID: "", SessionID: "s3", DomainID: "d1", Kind: "agent_invoke", CreatedAt: t0.Add(4 * time.Second)},
	}
	for _, rec := range recs {
		if err := r.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	res, err := r.ListSummaries(model.TraceListFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	if len(res.Summaries) != 2 {
		t.Fatalf("summaries = %d, want 2", len(res.Summaries))
	}
	// Most recent first: trace-B before trace-A.
	if res.Summaries[0].TraceID != "trace-B" {
		t.Fatalf("expected trace-B first, got %s", res.Summaries[0].TraceID)
	}
	a := res.Summaries[1]
	if a.TraceID != "trace-A" || a.SessionID != "s1" {
		t.Fatalf("trace-A summary wrong: %+v", a)
	}
	if a.ObservationCount != 3 {
		t.Fatalf("trace-A obs count = %d, want 3", a.ObservationCount)
	}
	if a.AgentInvocations != 1 || a.ToolInvocations != 1 {
		t.Fatalf("trace-A invocations wrong: %+v", a)
	}
	if a.TotalCostUSD != 0.03 || a.TotalPromptTokens != 5 || a.TotalCompletionTokens != 7 {
		t.Fatalf("trace-A rollup wrong: %+v", a)
	}
	if a.DurationMs != 2_000 {
		t.Fatalf("trace-A duration = %dms, want 2000", a.DurationMs)
	}
	if a.Status != "completed" {
		t.Fatalf("trace-A status = %q, want completed", a.Status)
	}
}

func TestInMemoryRepository_ListSummaries_Filter(t *testing.T) {
	r := NewInMemoryRepository()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	recs := []*model.ObservationRecord{
		{TraceID: "t-x", SessionID: "s-x", DomainID: "d1", AgentID: "billing", Kind: "agent_invoke", CreatedAt: t0},
		{TraceID: "t-y", SessionID: "s-y", DomainID: "d2", AgentID: "technical", Kind: "agent_invoke", CreatedAt: t0},
	}
	for _, rec := range recs {
		if err := r.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	cases := []struct {
		name   string
		filter model.TraceListFilter
		want   int
	}{
		{"domain d1", model.TraceListFilter{DomainID: "d1"}, 1},
		{"session s-y", model.TraceListFilter{SessionID: "s-y"}, 1},
		{"agent billing", model.TraceListFilter{AgentID: "billing"}, 1},
		{"from-only keeps both", model.TraceListFilter{From: t0.Add(-1 * time.Hour)}, 2},
		{"from in future", model.TraceListFilter{From: t0.Add(1 * time.Hour)}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := r.ListSummaries(tc.filter)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if res.Total != tc.want {
				t.Fatalf("total = %d, want %d", res.Total, tc.want)
			}
		})
	}
}

func TestInMemoryRepository_ListSummaries_Pagination(t *testing.T) {
	r := NewInMemoryRepository()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		_ = r.Save(&model.ObservationRecord{
			TraceID:   fmt.Sprintf("t-%02d", i),
			SessionID: fmt.Sprintf("s-%02d", i),
			Kind:      "agent_invoke",
			CreatedAt: t0.Add(time.Duration(i) * time.Second),
		})
	}

	page1, err := r.ListSummaries(model.TraceListFilter{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if page1.Total != 7 || len(page1.Summaries) != 3 {
		t.Fatalf("page1 total=%d len=%d", page1.Total, len(page1.Summaries))
	}
	if page1.Summaries[0].TraceID != "t-06" {
		t.Fatalf("page1[0] = %s, want t-06", page1.Summaries[0].TraceID)
	}
	page2, err := r.ListSummaries(model.TraceListFilter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if page2.Total != 7 || len(page2.Summaries) != 3 {
		t.Fatalf("page2 total=%d len=%d", page2.Total, len(page2.Summaries))
	}
	page3, err := r.ListSummaries(model.TraceListFilter{Limit: 3, Offset: 6})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if page3.Total != 7 || len(page3.Summaries) != 1 {
		t.Fatalf("page3 total=%d len=%d", page3.Total, len(page3.Summaries))
	}
}

func TestInMemoryRepository_Detail(t *testing.T) {
	r := NewInMemoryRepository()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	recs := []*model.ObservationRecord{
		{TraceID: "trace-X", SessionID: "sX", DomainID: "dX", Kind: "agent_invoke", CostUSD: 0.01, CreatedAt: t0},
		{TraceID: "trace-X", SessionID: "sX", DomainID: "dX", Kind: "tool_call", CostUSD: 0.02, CreatedAt: t0.Add(1 * time.Second)},
		// Different trace → must not be included.
		{TraceID: "trace-Y", SessionID: "sY", DomainID: "dX", Kind: "agent_invoke", CreatedAt: t0.Add(2 * time.Second)},
	}
	for _, rec := range recs {
		if err := r.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	d, err := r.Detail("trace-X")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.TraceID != "trace-X" || d.SessionID != "sX" {
		t.Fatalf("ids wrong: %+v", d.TraceSummary)
	}
	if len(d.Observations) != 2 {
		t.Fatalf("obs count = %d, want 2", len(d.Observations))
	}
	if d.TotalCostUSD != 0.03 || d.AgentInvocations != 1 || d.ToolInvocations != 1 {
		t.Fatalf("detail rollup wrong: %+v", d.TraceSummary)
	}

	// Missing trace must surface ErrTraceNotFound so the handler can map it
	// to a 404 with a stable code.
	if _, err := r.Detail("nonexistent"); !errors.Is(err, model.ErrTraceNotFound) {
		t.Fatalf("expected ErrTraceNotFound, got %v", err)
	}
}
