package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
