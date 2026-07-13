package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	approvalmodel "github.com/hnsx-io/hnsx/server/internal/approval/model"
	approvalrepo "github.com/hnsx-io/hnsx/server/internal/approval/repository"
	approvalservice "github.com/hnsx-io/hnsx/server/internal/approval/service"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
)

func newApprovalTestServer(t *testing.T) (*Server, *approvalservice.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := approvalservice.NewService(approvalrepo.NewInMemoryRepository(), nil)
	application := &app.Application{ApprovalService: svc}
	return &Server{ApprovalService: svc, Handlers: handler.New(application, nil)}, svc
}

func TestApprovals_ListFiltersByStatus(t *testing.T) {
	s, svc := newApprovalTestServer(t)

	for _, id := range []string{"a1", "a2"} {
		if err := svc.Create(&approvalmodel.Approval{ID: id, SessionID: "s1", Action: "tool:shell"}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	if _, err := svc.Approve("a2", "operator", "looks fine"); err != nil {
		t.Fatalf("approve a2: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	s.ListApprovals(c)
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
		t.Fatalf("expected 1 pending, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	if resp.Items[0]["id"] != "a1" {
		t.Fatalf("wrong id: %+v", resp.Items[0])
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/approvals?status=approved", nil)
	s.ListApprovals(c)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || resp.Items[0]["id"] != "a2" {
		t.Fatalf("approved list wrong: %+v", resp)
	}
}

func TestApprovals_ApproveRejects(t *testing.T) {
	s, svc := newApprovalTestServer(t)
	if err := svc.Create(&approvalmodel.Approval{
		ID:        "apr-1",
		SessionID: "sess-1",
		DomainID:  "billing",
		Action:    "tool:refund",
		Resource:  "tool:refund",
		RiskLevel: approvalmodel.RiskHigh,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"reviewed_by": "alice",
		"comment":     "verified refund policy",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/apr-1/approve", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "apr-1"}}
	s.ApproveApproval(c)
	if w.Code != http.StatusOK {
		t.Fatalf("approve status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "refund policy") {
		t.Fatalf("comment missing in response: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/approvals/apr-1/approve", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "apr-1"}}
	s.ApproveApproval(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestApprovals_Reject(t *testing.T) {
	s, svc := newApprovalTestServer(t)
	if err := svc.Create(&approvalmodel.Approval{ID: "apr-2", SessionID: "s2", Action: "tool:delete"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"reviewed_by": "bob", "comment": "too risky"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/apr-2/reject", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "apr-2"}}
	s.RejectApproval(c)
	if w.Code != http.StatusOK {
		t.Fatalf("reject status = %d", w.Code)
	}
	var resp struct {
		Status     string `json:"status"`
		ReviewedBy string `json:"reviewed_by"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "rejected" || resp.ReviewedBy != "bob" {
		t.Fatalf("decisions mismatch: %+v", resp)
	}
}

func TestApprovals_NotFound(t *testing.T) {
	s, _ := newApprovalTestServer(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/approvals/missing", nil)
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	s.GetApproval(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestApprovals_NilService(t *testing.T) {
	s := &Server{ApprovalService: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	s.ListApprovals(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestApprovals_GateBlocksUntilResolve(t *testing.T) {
	svc := approvalservice.NewService(approvalrepo.NewInMemoryRepository(), nil)
	a := &approvalmodel.Approval{
		ID:        "gate-1",
		SessionID: "sess-X",
		DomainID:  "billing",
		Action:    "tool:refund",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	var gotDecision approvalservice.Decision
	go func() {
		defer wg.Done()
		decision, _, err := svc.Request(ctx, a)
		if err == nil {
			gotDecision = decision
		}
	}()
	// Poll until the approval row lands so Request has had a chance
	// to register.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if _, err := svc.Get("gate-1"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approval never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := svc.Approve("gate-1", "operator", "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	wg.Wait()
	if gotDecision != approvalservice.DecisionApproved {
		t.Fatalf("gate decision = %s, want approved", gotDecision)
	}
}
