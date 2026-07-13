package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	tracerepo "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
)

func newTraceTestServer(t *testing.T) (*Server, tracerepo.Repository) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	trRepo := tracerepo.NewInMemoryRepository()
	trSvc := traceservice.NewService(trRepo)
	application := &app.Application{TraceService: trSvc}

	s := &Server{
		TraceService: trSvc,
		Handlers:     handler.New(application, nil),
	}
	return s, trRepo
}

func TestListTraces_AggregatesByTraceID(t *testing.T) {
	s, trRepo := newTraceTestServer(t)

	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	recs := []*tracemodel.ObservationRecord{
		{TraceID: "trace-A", SessionID: "s1", DomainID: "d1", DomainVersion: "1.0.0",
			Kind: "agent_invoke", CostUSD: 0.02, PromptTokens: 3, CompletionTokens: 4, CreatedAt: t0},
		{TraceID: "trace-A", SessionID: "s1", DomainID: "d1", DomainVersion: "1.0.0",
			Kind: "tool_call", CostUSD: 0.01, CreatedAt: t0.Add(1 * time.Second)},
		// Different trace_id must produce a separate row, even if SessionID
		// is reused.
		{TraceID: "trace-B", SessionID: "s1", DomainID: "d1", DomainVersion: "1.0.0",
			Kind: "agent_invoke", CostUSD: 0.10, CreatedAt: t0.Add(2 * time.Second)},
	}
	for _, rec := range recs {
		if err := trRepo.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=10", nil)

	s.ListTraces(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected 2 traces, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	if resp.Limit != 10 {
		t.Fatalf("limit = %d, want 10", resp.Limit)
	}
	// Most recent first by started_at — trace-B is the newest.
	if resp.Items[0]["trace_id"] != "trace-B" {
		t.Fatalf("first item = %v, want trace-B", resp.Items[0]["trace_id"])
	}
	got := resp.Items[1]
	if got["trace_id"] != "trace-A" || got["session_id"] != "s1" {
		t.Fatalf("trace-A shape wrong: %+v", got)
	}
	if v := got["total_cost_usd"].(float64); v < 0.0299 || v > 0.0301 {
		t.Fatalf("trace-A total_cost_usd = %v, want ~0.03", v)
	}
	if v := got["agent_invocations"].(float64); v != 1 {
		t.Fatalf("trace-A agent_invocations = %v, want 1", v)
	}
	if v := got["tool_invocations"].(float64); v != 1 {
		t.Fatalf("trace-A tool_invocations = %v, want 1", v)
	}
}

func TestListTraces_FilterByDomain(t *testing.T) {
	s, trRepo := newTraceTestServer(t)
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	for _, rec := range []*tracemodel.ObservationRecord{
		{TraceID: "t-1", SessionID: "s1", DomainID: "billing", Kind: "agent_invoke", CreatedAt: t0},
		{TraceID: "t-2", SessionID: "s2", DomainID: "tech", Kind: "agent_invoke", CreatedAt: t0.Add(time.Second)},
	} {
		if err := trRepo.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces?domain=tech", nil)
	s.ListTraces(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Total int              `json:"total"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 trace for tech, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	if resp.Items[0]["domain_id"] != "tech" {
		t.Fatalf("filter leak: %+v", resp.Items[0])
	}
}

func TestGetTrace_ReturnsObservationsAndRollup(t *testing.T) {
	s, trRepo := newTraceTestServer(t)
	t0 := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	for _, rec := range []*tracemodel.ObservationRecord{
		{TraceID: "trace-X", SessionID: "sX", DomainID: "d1", DomainVersion: "1",
			Kind: "agent_invoke", CostUSD: 0.05, CreatedAt: t0},
		{TraceID: "trace-X", SessionID: "sX", DomainID: "d1", DomainVersion: "1",
			Kind: "tool_call", CostUSD: 0.02, CreatedAt: t0.Add(time.Second)},
		// Different trace_id must not bleed into the response.
		{TraceID: "trace-Y", SessionID: "sY", DomainID: "d1", Kind: "agent_invoke", CreatedAt: t0.Add(2 * time.Second)},
	} {
		if err := trRepo.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-X", nil)
	c.Params = gin.Params{{Key: "traceId", Value: "trace-X"}}

	s.GetTrace(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		TraceID          string           `json:"trace_id"`
		SessionID        string           `json:"session_id"`
		DomainID         string           `json:"domain_id"`
		Status           string           `json:"status"`
		TotalCostUSD     float64          `json:"total_cost_usd"`
		AgentInvocations int              `json:"agent_invocations"`
		ToolInvocations  int              `json:"tool_invocations"`
		ObservationCount int              `json:"observation_count"`
		DurationMs       int64            `json:"duration_ms"`
		Observations     []map[string]any `json:"observations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TraceID != "trace-X" || resp.SessionID != "sX" || resp.DomainID != "d1" {
		t.Fatalf("ids wrong: %+v", resp)
	}
	if resp.TotalCostUSD < 0.0699 || resp.TotalCostUSD > 0.0701 {
		t.Fatalf("total_cost_usd = %v, want ~0.07", resp.TotalCostUSD)
	}
	if resp.AgentInvocations != 1 || resp.ToolInvocations != 1 {
		t.Fatalf("invocations wrong: %+v", resp)
	}
	if resp.ObservationCount != 2 {
		t.Fatalf("observation_count = %d, want 2", resp.ObservationCount)
	}
	if resp.DurationMs != 1000 {
		t.Fatalf("duration_ms = %d, want 1000", resp.DurationMs)
	}
	if len(resp.Observations) != 2 {
		t.Fatalf("len(observations) = %d, want 2 (trace-Y must be excluded)", len(resp.Observations))
	}
	if resp.Observations[0]["trace_id"] != "trace-X" {
		t.Fatalf("observation trace_id leaked: %+v", resp.Observations[0])
	}
}

func TestGetTrace_NotFound(t *testing.T) {
	s, _ := newTraceTestServer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces/missing", nil)
	c.Params = gin.Params{{Key: "traceId", Value: "missing"}}

	s.GetTrace(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != "TRACE_NOT_FOUND" {
		t.Fatalf("error code = %q, want TRACE_NOT_FOUND", resp.Code)
	}
}
