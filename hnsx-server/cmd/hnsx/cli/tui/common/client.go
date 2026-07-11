package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

// Client wraps the internal client with TUI-specific helpers. It never
// re-implements protocol logic; it only adapts data for tab views.
type Client struct {
	*client.Client
}

// NewClient creates a TUI client from a server base URL.
func NewClient(serverURL string) *Client {
	return &Client{Client: client.NewWithBaseURL(serverURL)}
}

// TraceListItem is a single row in the Traces tab.
type TraceListItem struct {
	ID         string  `json:"trace_id"`
	SessionID  string  `json:"session_id"`
	DomainID   string  `json:"domain_id"`
	StartedAt  string  `json:"started_at"`
	DurationMS int64   `json:"duration_ms"`
	Cost       float64 `json:"total_cost_usd"`
}

// ListTraces fetches the trace list from the REST API.
func (c *Client) ListTraces() ([]TraceListItem, error) {
	body, err := c.get("/api/v1/traces")
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []TraceListItem `json:"items"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse traces: %w", err)
	}
	if env.Items == nil {
		return []TraceListItem{}, nil
	}
	return env.Items, nil
}

// GetTrace fetches a single trace by ID and returns the raw JSON tree.
func (c *Client) GetTrace(id string) (map[string]any, error) {
	body, err := c.get("/api/v1/traces/" + id)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse trace: %w", err)
	}
	return out, nil
}

func (c *Client) get(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// StreamSessionEvents delegates to the underlying client and forwards events
// as a bubbletea command-compatible channel.
func (c *Client) StreamSessionEvents(ctx context.Context, id string) (
	<-chan client.Event, <-chan error, error,
) {
	return c.Client.SessionEvents(ctx, id)
}

// ListSessions delegates to the underlying client.
func (c *Client) ListSessions() ([]client.SessionListItem, error) {
	return c.Client.ListSessions()
}

// GetSession delegates to the underlying client.
func (c *Client) GetSession(id string) (*client.Session, error) {
	return c.Client.GetSession(id)
}

// RerunSession delegates to the underlying client.
func (c *Client) RerunSession(id string) (*client.Session, error) {
	return c.Client.RerunSession(id)
}

// CancelSession delegates to the underlying client.
func (c *Client) CancelSession(id string) (*client.Session, error) {
	return c.Client.CancelSession(id)
}

// ApprovalItem is a single pending approval.
type ApprovalItem struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Risk      string `json:"risk"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// ListApprovals fetches pending approvals from the REST API.
func (c *Client) ListApprovals() ([]ApprovalItem, error) {
	body, err := c.get("/api/v1/approvals")
	if err != nil {
		return nil, err
	}
	items, err := parseListEnvelope(body)
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalItem, 0, len(items))
	for _, it := range items {
		out = append(out, ApprovalItem{
			ID:        stringOr(it["id"], ""),
			SessionID: stringOr(it["session_id"], ""),
			Risk:      stringOr(it["risk"], ""),
			Reason:    stringOr(it["reason"], ""),
			CreatedAt: stringOr(it["created_at"], ""),
		})
	}
	return out, nil
}

// ApproveApproval POSTs an approve action.
func (c *Client) ApproveApproval(id string) error {
	return c.postNoBody("/api/v1/approvals/" + id + "/approve")
}

// RejectApproval POSTs a reject action with an optional reason.
func (c *Client) RejectApproval(id, reason string) error {
	payload := map[string]any{}
	if reason != "" {
		payload["reason"] = reason
	}
	var body io.Reader
	if len(payload) > 0 {
		b, _ := json.Marshal(payload)
		body = strings.NewReader(string(b))
	}
	return c.postJSON("/api/v1/approvals/"+id+"/reject", body)
}

// ListEvalSets delegates to the underlying client.
func (c *Client) ListEvalSets() ([]client.EvalSet, error) {
	return c.Client.ListEvalSets()
}

// ListEvalRuns delegates to the underlying client.
func (c *Client) ListEvalRuns(setID string) ([]client.EvalRun, error) {
	return c.Client.ListEvalRuns(setID)
}

// GetEvalRun delegates to the underlying client.
func (c *Client) GetEvalRun(setID, runID string) (*client.EvalRun, error) {
	return c.Client.GetEvalRun(setID, runID)
}

// RunEval delegates to the underlying client.
func (c *Client) RunEval(setID string) (*client.EvalRun, error) {
	return c.Client.RunEval(setID)
}

// ListDomains delegates to the underlying client.
func (c *Client) ListDomains() ([]client.DomainListItem, error) {
	return c.Client.ListDomains()
}

// Health returns true if the server root endpoint responds with a 2xx/3xx status.
func (c *Client) Health() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode/100 == 2 || resp.StatusCode/100 == 3
}

// GetDomain delegates to the underlying client.
func (c *Client) GetDomain(id string) (*client.Domain, error) {
	return c.Client.GetDomain(id)
}

// TriggerSession delegates to the underlying client.
func (c *Client) TriggerSession(domainID string, trigger map[string]any) (*client.Session, error) {
	return c.Client.TriggerSession(domainID, trigger)
}

// AuditItem is a single audit log entry.
type AuditItem struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Actor     string `json:"actor"`
	Resource  string `json:"resource"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
}

// ListAudit fetches audit log entries from the REST API.
func (c *Client) ListAudit() ([]AuditItem, error) {
	body, err := c.get("/api/v1/audit")
	if err != nil {
		return nil, err
	}
	items, err := parseListEnvelope(body)
	if err != nil {
		return nil, err
	}
	out := make([]AuditItem, 0, len(items))
	for _, it := range items {
		out = append(out, AuditItem{
			Timestamp: stringOr(it["timestamp"], ""),
			Action:    stringOr(it["action"], ""),
			Actor:     stringOr(it["actor"], ""),
			Resource:  stringOr(it["resource"], ""),
			Decision:  stringOr(it["decision"], ""),
			Reason:    stringOr(it["reason"], ""),
		})
	}
	return out, nil
}

// DashboardSummary aggregates lightweight metrics from sessions and traces.
type DashboardSummary struct {
	PendingApprovals int
	RunningSessions  int
	TotalSessions24h int
	Cost24h          float64
	FailureRate      float64
}

// DashboardSummary fetches sessions and traces and derives dashboard cards.
func (c *Client) DashboardSummary() (*DashboardSummary, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}
	traces, err := c.ListTraces()
	if err != nil {
		return nil, err
	}

	summary := &DashboardSummary{}
	cutoff := time.Now().Add(-24 * time.Hour)
	var failed int
	for _, s := range sessions {
		if s.State == "running" {
			summary.RunningSessions++
		}
		if t, err := time.Parse(time.RFC3339, s.StartedAt); err == nil && t.After(cutoff) {
			summary.TotalSessions24h++
			if s.State == "failed" {
				failed++
			}
		}
	}
	for _, tr := range traces {
		if t, err := time.Parse(time.RFC3339, tr.StartedAt); err == nil && t.After(cutoff) {
			summary.Cost24h += tr.Cost
		}
	}
	if summary.TotalSessions24h > 0 {
		summary.FailureRate = float64(failed) / float64(summary.TotalSessions24h)
	}
	return summary, nil
}

func (c *Client) postNoBody(path string) error {
	return c.postJSON(path, nil)
}

func (c *Client) postJSON(path string, body io.Reader) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func parseListEnvelope(body []byte) ([]map[string]any, error) {
	var env struct {
		Items []map[string]any `json:"items"`
		Data  []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Items != nil {
		return env.Items, nil
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Data != nil {
		return env.Data, nil
	}
	var out []map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse list: %w", err)
	}
	return out, nil
}

func stringOr(v any, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

