package hnsx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hnsx-io/hnsx/sdk/go"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/domains":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{{"id": "customer-service", "version": "1.0.0", "status": "active"}},
					"total": 1,
				})
				return
			}
		case "/api/v1/sessions":
			if r.Method == http.MethodPost {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":        "sess-123",
					"domain_id": body["domain_id"],
					"state":     "running",
					"trigger":   body["trigger"],
				})
				return
			}
		case "/api/v1/approvals/approve-1/approve":
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "approve-1", "status": "approved"})
				return
			}
		case "/api/v1/domains/missing":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "DOMAIN_NOT_FOUND", "message": "not found"})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestClient_DomainsList(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client := hnsx.NewClient(server.URL)
	resp, err := client.Domains.List(context.Background(), 50, 0)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Items[0].ID != "customer-service" {
		t.Fatalf("unexpected id: %s", resp.Items[0].ID)
	}
}

func TestClient_SessionsTrigger(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client := hnsx.NewClient(server.URL)
	session, err := client.Sessions.Trigger(context.Background(), "customer-service", map[string]any{"question": "hi"})
	if err != nil {
		t.Fatalf("trigger session: %v", err)
	}
	if session.ID != "sess-123" {
		t.Fatalf("unexpected session id: %s", session.ID)
	}
	if session.DomainID != "customer-service" {
		t.Fatalf("unexpected domain id: %s", session.DomainID)
	}
}

func TestClient_ApprovalsApprove(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client := hnsx.NewClient(server.URL)
	approval, err := client.Approvals.Approve(context.Background(), "approve-1")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approval.Status != "approved" {
		t.Fatalf("unexpected status: %s", approval.Status)
	}
}

func TestClient_APIError(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client := hnsx.NewClient(server.URL)
	_, err := client.Domains.Get(context.Background(), "missing")
	apiErr, ok := err.(*hnsx.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "DOMAIN_NOT_FOUND" {
		t.Fatalf("unexpected code: %s", apiErr.Code)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", apiErr.Status)
	}
}
