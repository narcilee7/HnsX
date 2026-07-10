package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrepo "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
)

func newEvalTestServer(t *testing.T) (*Server, *evalservice.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	return &Server{EvalService: svc}, svc
}

func seedEvalSet(t *testing.T, svc *evalservice.Service) *evalmodel.EvalSet {
	t.Helper()
	set := &evalmodel.EvalSet{
		ID:          "routing-accuracy",
		SetID:       "routing-accuracy",
		DomainID:    "customer-service",
		Description: "initial",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Name: "billing question"},
		},
	}
	if err := svc.CreateSet(set); err != nil {
		t.Fatalf("seed set: %v", err)
	}
	return set
}

func TestEvals_UpdateSet(t *testing.T) {
	s, svc := newEvalTestServer(t)
	seedEvalSet(t, svc)

	body, _ := json.Marshal(map[string]any{
		"description": "updated",
		"cases": []map[string]any{
			{"id": "c2", "name": "technical question"},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/evals/routing-accuracy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "setId", Value: "routing-accuracy"}}
	s.UpdateEvalSet(c)

	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d; body=%s", w.Code, w.Body.String())
	}

	updated, err := svc.GetSet("routing-accuracy")
	if err != nil {
		t.Fatalf("get updated set: %v", err)
	}
	if updated.Description != "updated" {
		t.Fatalf("description not updated: %s", updated.Description)
	}
	if len(updated.Cases) != 1 || updated.Cases[0].ID != "c2" {
		t.Fatalf("cases not replaced: %+v", updated.Cases)
	}
}

func TestEvals_UpdateSet_NotFound(t *testing.T) {
	s, _ := newEvalTestServer(t)
	body, _ := json.Marshal(map[string]any{"description": "updated"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/evals/missing", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "setId", Value: "missing"}}
	s.UpdateEvalSet(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestEvals_DeleteSet(t *testing.T) {
	s, svc := newEvalTestServer(t)
	seedEvalSet(t, svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/evals/routing-accuracy", nil)
	c.Params = gin.Params{{Key: "setId", Value: "routing-accuracy"}}
	s.DeleteEvalSet(c)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", w.Code)
	}

	if _, err := svc.GetSet("routing-accuracy"); err == nil {
		t.Fatal("expected set to be deleted")
	}
}

func TestEvals_DeleteSet_NotFound(t *testing.T) {
	s, _ := newEvalTestServer(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/evals/missing", nil)
	c.Params = gin.Params{{Key: "setId", Value: "missing"}}
	s.DeleteEvalSet(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestEvals_ListRuns(t *testing.T) {
	s, svc := newEvalTestServer(t)
	set := seedEvalSet(t, svc)

	for i := 0; i < 3; i++ {
		run := &evalmodel.EvalRun{
			ID:        set.ID + "-run-" + string(rune('a'+i)),
			EvalSetID: set.ID,
			DomainID:  set.DomainID,
			State:     "completed",
			Score:     0.9,
		}
		if err := svc.CreateRun(run); err != nil {
			t.Fatalf("seed run: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/evals/routing-accuracy/runs", nil)
	c.Params = gin.Params{{Key: "setId", Value: "routing-accuracy"}}
	s.ListEvalRuns(c)

	if w.Code != http.StatusOK {
		t.Fatalf("list runs status = %d; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected 3 runs, got %d", resp.Total)
	}
}

func TestEvals_ListRuns_SetNotFound(t *testing.T) {
	s, _ := newEvalTestServer(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/evals/missing/runs", nil)
	c.Params = gin.Params{{Key: "setId", Value: "missing"}}
	s.ListEvalRuns(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestEvals_NilService(t *testing.T) {
	s := &Server{EvalService: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/evals/missing/runs", nil)
	c.Params = gin.Params{{Key: "setId", Value: "missing"}}
	s.ListEvalRuns(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
