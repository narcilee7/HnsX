package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	sessionrepo "github.com/hnsx-io/hnsx/server/internal/session/repository"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	tracerepo "github.com/hnsx-io/hnsx/server/internal/trace/repository"
	traceservice "github.com/hnsx-io/hnsx/server/internal/trace/service"
)

func newTraceTestServer(t *testing.T) (*Server, *sessionservice.Service, tracerepo.Repository) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	sessSvc := sessionservice.NewService(sessionrepo.NewInMemoryRepository())
	trRepo := tracerepo.NewInMemoryRepository()
	trSvc := traceservice.NewService(trRepo)

	s := &Server{
		Queries:      queries.NewQueries(nil, sessSvc),
		TraceService: trSvc,
	}
	return s, sessSvc, trRepo
}

func TestListTraces_EnrichesWithAggregate(t *testing.T) {
	s, sessSvc, trRepo := newTraceTestServer(t)

	if _, err := sessSvc.Create(sessionservice.CreateParams{
		SessionID: "s1", DomainID: "d1", DomainVersion: "1.0.0", Orchestration: "single",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = trRepo.Save(&tracemodel.ObservationRecord{
		SessionID: "s1", TraceID: "s1", Kind: "agent_invoke",
		CostUSD: 0.02, PromptTokens: 3, CompletionTokens: 4, CreatedAt: time.Now().UTC(),
	})
	_ = trRepo.Save(&tracemodel.ObservationRecord{
		SessionID: "s1", TraceID: "s1", Kind: "tool_call",
		CostUSD: 0.01, CreatedAt: time.Now().UTC(),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)

	s.ListTraces(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 trace, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	item := resp.Items[0]
	if item["trace_id"] != "s1" || item["session_id"] != "s1" {
		t.Fatalf("trace/session id wrong: %+v", item)
	}
	if got := item["total_cost_usd"].(float64); got < 0.0299 || got > 0.0301 {
		t.Fatalf("total_cost_usd = %v, want ~0.03", got)
	}
	if item["agent_invocations"].(float64) != 1 || item["tool_invocations"].(float64) != 1 {
		t.Fatalf("invocations wrong: %+v", item)
	}
}

func TestGetTrace_ReturnsObservations(t *testing.T) {
	s, sessSvc, trRepo := newTraceTestServer(t)

	if _, err := sessSvc.Create(sessionservice.CreateParams{
		SessionID: "s1", DomainID: "d1", DomainVersion: "1.0.0", Orchestration: "single",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = trRepo.Save(&tracemodel.ObservationRecord{
		SessionID: "s1", TraceID: "s1", Kind: "agent_invoke",
		CostUSD: 0.05, CreatedAt: time.Now().UTC(),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces/s1", nil)
	c.Params = gin.Params{{Key: "traceId", Value: "s1"}}

	s.GetTrace(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		TraceID      string           `json:"trace_id"`
		SessionID    string           `json:"session_id"`
		Observations []map[string]any `json:"observations"`
		Summary      map[string]any   `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TraceID != "s1" || resp.SessionID != "s1" {
		t.Fatalf("ids wrong: %+v", resp)
	}
	if len(resp.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(resp.Observations))
	}
	if resp.Observations[0]["kind"] != "agent_invoke" {
		t.Fatalf("observation kind wrong: %+v", resp.Observations[0])
	}
	if got := resp.Summary["total_cost_usd"].(float64); got < 0.0499 || got > 0.0501 {
		t.Fatalf("summary cost = %v, want ~0.05", got)
	}
}

func TestGetTrace_NotFound(t *testing.T) {
	s, _, _ := newTraceTestServer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces/missing", nil)
	c.Params = gin.Params{{Key: "traceId", Value: "missing"}}

	s.GetTrace(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
