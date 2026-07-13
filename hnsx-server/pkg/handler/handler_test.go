package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	domainrepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrepo "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	tracerepo "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func testTenant() tenant.ID { return tenant.ID("tenant-a") }

func testDomainSpec(id, version string) *domain.DomainSpec {
	return &domain.DomainSpec{
		ID:          id,
		Version:     version,
		Description: "test domain",
		Harness: domain.HarnessSpec{
			Agents: map[string]domain.AgentSpec{
				"agent-1": {
					ID:       "agent-1",
					Provider: "echo",
					Adapter:  domain.AdapterConfig{Kind: "echo"},
				},
			},
			Sandbox: domain.SandboxSpec{Policy: "none"},
			Session: domain.SessionSpec{Mode: domain.Single, Agent: "agent-1"},
		},
	}
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	domainSvc := domainservice.NewService(domainrepo.NewInMemoryRepository())
	evalSvc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	traceSvc := traceservice.NewService(tracerepo.NewInMemoryRepository())
	application := &app.Application{
		DomainService: domainSvc,
		EvalService:   evalSvc,
		TraceService:  traceSvc,
	}
	return New(application, nil)
}

func TestDomain_List(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	tid := testTenant()

	if _, err := h.App.DomainService.Register(tid, testDomainSpec("alpha", "1.0.0")); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if _, err := h.App.DomainService.Register(tid, testDomainSpec("beta", "1.0.0")); err != nil {
		t.Fatalf("register beta: %v", err)
	}

	out, err := h.ListDomains(ctx, ListDomainsInput{TenantID: tid, Limit: 10})
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if out.Domains.Total != 2 {
		t.Fatalf("total = %d, want 2", out.Domains.Total)
	}
	if out.Domains.Items[0].ID != "alpha" || out.Domains.Items[1].ID != "beta" {
		t.Fatalf("sort order wrong: %+v", out.Domains.Items)
	}
}

func TestDomain_Get(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	tid := testTenant()

	if _, err := h.App.DomainService.Register(tid, testDomainSpec("found", "1.0.0")); err != nil {
		t.Fatalf("register: %v", err)
	}

	out, err := h.GetDomain(ctx, GetDomainInput{TenantID: tid, ID: "found"})
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if out.Domain.ID != "found" {
		t.Fatalf("id = %q, want found", out.Domain.ID)
	}
}

func TestDomain_Get_NotFound(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	_, err := h.GetDomain(ctx, GetDomainInput{TenantID: testTenant(), ID: "missing"})
	if !errors.Is(err, domainmodel.ErrDomainNotFound) && !IsDomainNotFound(err) {
		t.Fatalf("expected domain not found, got %v", err)
	}
}

func TestDomain_Delete(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	tid := testTenant()

	if _, err := h.App.DomainService.Register(tid, testDomainSpec("gone", "1.0.0")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := h.DeleteDomain(ctx, DeleteDomainInput{TenantID: tid, ID: "gone"}); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}
	if _, err := h.GetDomain(ctx, GetDomainInput{TenantID: tid, ID: "gone"}); !IsDomainNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestEval_ListRuns(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	set := &evalmodel.EvalSet{ID: "es-1", SetID: "es-1", DomainID: "d-1"}
	if err := h.App.EvalService.CreateSet(set); err != nil {
		t.Fatalf("create set: %v", err)
	}
	for _, rid := range []string{"r-1", "r-2"} {
		if err := h.App.EvalService.CreateRun(&evalmodel.EvalRun{ID: rid, EvalSetID: set.ID, State: "completed"}); err != nil {
			t.Fatalf("create run %s: %v", rid, err)
		}
	}

	out, err := h.ListEvalRuns(ctx, ListEvalRunsInput{TenantID: testTenant(), SetID: set.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListEvalRuns: %v", err)
	}
	if out.Runs.Total != 2 {
		t.Fatalf("total = %d, want 2", out.Runs.Total)
	}
}

func TestEval_GetRun(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	set := &evalmodel.EvalSet{ID: "es-1", SetID: "es-1", DomainID: "d-1"}
	if err := h.App.EvalService.CreateSet(set); err != nil {
		t.Fatalf("create set: %v", err)
	}
	run := &evalmodel.EvalRun{ID: "r-1", EvalSetID: set.ID, State: "completed", Score: 0.75}
	if err := h.App.EvalService.CreateRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	out, err := h.GetEvalRun(ctx, GetEvalRunInput{TenantID: testTenant(), RunID: run.ID})
	if err != nil {
		t.Fatalf("GetEvalRun: %v", err)
	}
	if out.Run.ID != run.ID {
		t.Fatalf("id = %q, want %q", out.Run.ID, run.ID)
	}
	if out.Run.State != "completed" {
		t.Fatalf("state = %q, want completed", out.Run.State)
	}
}

func TestTrace_List(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	repo := tracerepo.NewInMemoryRepository()
	traceSvc := traceservice.NewService(repo)
	h.App.TraceService = traceSvc

	for _, rec := range []*tracemodel.ObservationRecord{
		{TraceID: "t-1", SessionID: "s-1", DomainID: "d-1", DomainVersion: "1", Kind: "agent_invoke", CostUSD: 0.01, CreatedAt: t0},
		{TraceID: "t-1", SessionID: "s-1", DomainID: "d-1", DomainVersion: "1", Kind: "tool_call", CostUSD: 0.02, CreatedAt: t0.Add(time.Second)},
		{TraceID: "t-2", SessionID: "s-2", DomainID: "d-1", DomainVersion: "1", Kind: "agent_invoke", CostUSD: 0.10, CreatedAt: t0.Add(2 * time.Second)},
	} {
		if err := repo.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	out, err := h.ListTraces(ctx, ListTracesInput{TenantID: testTenant(), Limit: 10})
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if out.Traces.Total != 2 {
		t.Fatalf("total = %d, want 2", out.Traces.Total)
	}
	// Newest trace first.
	if out.Traces.Items[0].TraceID != "t-2" {
		t.Fatalf("order wrong: %+v", out.Traces.Items)
	}
	// Aggregate fields propagate.
	if out.Traces.Items[1].TotalCostUSD < 0.029 || out.Traces.Items[1].TotalCostUSD > 0.031 {
		t.Fatalf("t-1 cost = %v, want ~0.03", out.Traces.Items[1].TotalCostUSD)
	}
	if out.Traces.Items[1].AgentInvocations != 1 || out.Traces.Items[1].ToolInvocations != 1 {
		t.Fatalf("t-1 invocations wrong: %+v", out.Traces.Items[1])
	}
}

func TestTrace_Get(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	repo := tracerepo.NewInMemoryRepository()
	traceSvc := traceservice.NewService(repo)
	h.App.TraceService = traceSvc

	for _, rec := range []*tracemodel.ObservationRecord{
		{TraceID: "t-1", SessionID: "s-1", DomainID: "d-1", DomainVersion: "1", Kind: "agent_invoke", CostUSD: 0.05, CreatedAt: t0},
		{TraceID: "t-1", SessionID: "s-1", DomainID: "d-1", DomainVersion: "1", Kind: "tool_call", CostUSD: 0.02, CreatedAt: t0.Add(time.Second)},
	} {
		if err := repo.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	out, err := h.GetTrace(ctx, GetTraceInput{TenantID: testTenant(), TraceID: "t-1"})
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if out.Trace.TraceID != "t-1" {
		t.Fatalf("trace_id = %q, want t-1", out.Trace.TraceID)
	}
	if len(out.Trace.Observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(out.Trace.Observations))
	}
	if out.Trace.TotalCostUSD < 0.069 || out.Trace.TotalCostUSD > 0.071 {
		t.Fatalf("cost = %v, want ~0.07", out.Trace.TotalCostUSD)
	}
}
