package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
	policyrepo "github.com/hnsx-io/hnsx/server/internal/policy/repository"
	policyservice "github.com/hnsx-io/hnsx/server/internal/policy/service"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
)

func newPolicyTestServer(t *testing.T) (*Server, *policyservice.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := policyservice.NewService(policyrepo.NewInMemoryRepository())
	application := &app.Application{PolicyService: svc}
	return &Server{PolicyService: svc, Handlers: handler.New(application, nil)}, svc
}

func doJSON(t *testing.T, method, path, body string, c *gin.Context, h gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	h(c)
	return w
}

func TestPolicies_ListEmpty(t *testing.T) {
	s, _ := newPolicyTestServer(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	s.ListPolicies(c)

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
	if resp.Total != 0 {
		t.Fatalf("expected 0, got %d", resp.Total)
	}
}

func TestPolicies_CRUD_Bind(t *testing.T) {
	s, svc := newPolicyTestServer(t)

	// Create.
	body, _ := json.Marshal(map[string]any{
		"id":          "no-shell",
		"name":        "Deny Shell",
		"description": "blocks shell tool",
		"budget":      map[string]any{"max_cost_usd": 10.0, "max_turns": 50},
		"permissions": map[string]any{"allow_shell": false},
		"guardrails": []map[string]any{
			{"id": "g1", "type": "keyword", "action": "block"},
		},
	})
	w := doJSON(t, http.MethodPost, "/api/v1/policies", string(body), nil, s.CreatePolicy)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", w.Code, w.Body.String())
	}

	// List should show 1 item with the rules.
	w = httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	s.ListPolicies(c)
	var listResp struct {
		Items []map[string]any
		Total int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != 1 {
		t.Fatalf("expected 1 policy, got %d", listResp.Total)
	}
	if listResp.Items[0]["id"] != "no-shell" {
		t.Fatalf("id mismatch: %+v", listResp.Items[0])
	}

	// Update via PUT — keeps the id, swaps the description.
	body, _ = json.Marshal(map[string]any{
		"name":        "Deny Shell (renamed)",
		"description": "blocks shell + denies file deletion",
	})
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/policies/no-shell", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "no-shell"}}
	s.UpdatePolicy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d; body=%s", w.Code, w.Body.String())
	}

	// Bind to a domain.
	body, _ = json.Marshal(map[string]any{"policy_id": "no-shell"})
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/domains/billing/policies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "billing"}}
	s.BindPolicy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("bind status = %d; body=%s", w.Code, w.Body.String())
	}

	// Verify the policy is now visible via repo ByDomain.
	got, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].BoundDomain != "billing" {
		t.Fatalf("binding missing: %+v", got)
	}

	// Delete.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/policies/no-shell", nil)
	c.Params = gin.Params{{Key: "id", Value: "no-shell"}}
	s.DeletePolicy(c)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", w.Code)
	}

	// Deleting again must surface POLICY_NOT_FOUND.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/policies/no-shell", nil)
	c.Params = gin.Params{{Key: "id", Value: "no-shell"}}
	s.DeletePolicy(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", w.Code)
	}
}

func TestPolicies_BindOneToOne(t *testing.T) {
	s, svc := newPolicyTestServer(t)
	for _, id := range []string{"p1", "p2"} {
		body, _ := json.Marshal(map[string]any{"id": id, "name": id})
		w := doJSON(t, http.MethodPost, "/api/v1/policies", string(body), nil, s.CreatePolicy)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d", id, w.Code)
		}
	}
	for _, id := range []string{"p1", "p2"} {
		body, _ := json.Marshal(map[string]any{"policy_id": id})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/domains/billing/policies", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req
		c.Params = gin.Params{{Key: "id", Value: "billing"}}
		s.BindPolicy(c)
		if w.Code != http.StatusOK {
			t.Fatalf("bind %s: %d; body=%s", id, w.Code, w.Body.String())
		}
	}
	// The most recent bind wins; p2 holds the slot, p1 unbound.
	items, _ := svc.List()
	var p1, p2 *policymodel.ListItem
	for i := range items {
		switch items[i].ID {
		case "p1":
			p1 = &items[i]
		case "p2":
			p2 = &items[i]
		}
	}
	if p1.BoundDomain != "" {
		t.Fatalf("p1 still bound: %+v", p1)
	}
	if p2.BoundDomain != "billing" {
		t.Fatalf("p2 not bound: %+v", p2)
	}
}

func TestPolicies_BindUnknownPolicy(t *testing.T) {
	s, _ := newPolicyTestServer(t)
	body, _ := json.Marshal(map[string]any{"policy_id": "missing"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains/billing/policies", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "billing"}}
	s.BindPolicy(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestPolicies_NilService(t *testing.T) {
	s := &Server{PolicyService: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	s.ListPolicies(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestPolicies_CreateRequiresID(t *testing.T) {
	s, _ := newPolicyTestServer(t)
	body, _ := json.Marshal(map[string]any{"name": "no id"})
	w := doJSON(t, http.MethodPost, "/api/v1/policies", string(body), nil, s.CreatePolicy)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
