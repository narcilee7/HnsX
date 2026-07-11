package hnsx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// DomainClient operates on domains.
type DomainClient struct {
	client *Client
}

// List returns all registered domains.
func (c *DomainClient) List(ctx context.Context, limit, offset int) (*ListEnvelope[DomainSummary], error) {
	path := "/domains" + queryString(map[string]any{"limit": limit, "offset": offset})
	data, err := c.client.doJSON(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out ListEnvelope[DomainSummary]
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a single domain by ID.
func (c *DomainClient) Get(ctx context.Context, id string) (*Domain, error) {
	data, err := c.client.doJSON(ctx, "GET", fmt.Sprintf("/domains/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var out Domain
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RegisterYAML uploads a domain YAML and returns the registered domain.
func (c *DomainClient) RegisterYAML(ctx context.Context, yaml string) (*Domain, error) {
	resp, err := c.client.request(ctx, "POST", "/domains", bytes.NewBufferString(yaml), map[string]string{"Content-Type": "application/x-yaml"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := decodeResponse(resp)
	if err != nil {
		return nil, err
	}
	var out Domain
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SessionClient operates on sessions.
type SessionClient struct {
	client *Client
}

// List returns sessions.
func (c *SessionClient) List(ctx context.Context, domain, state string, limit, offset int) (*ListEnvelope[SessionSummary], error) {
	path := "/sessions" + queryString(map[string]any{"domain": domain, "state": state, "limit": limit, "offset": offset})
	data, err := c.client.doJSON(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out ListEnvelope[SessionSummary]
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a single session by ID.
func (c *SessionClient) Get(ctx context.Context, id string) (*Session, error) {
	data, err := c.client.doJSON(ctx, "GET", fmt.Sprintf("/sessions/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var out Session
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Trigger starts a new session for a domain.
func (c *SessionClient) Trigger(ctx context.Context, domainID string, trigger map[string]any) (*Session, error) {
	data, err := c.client.doJSON(ctx, "POST", "/sessions", map[string]any{"domain_id": domainID, "trigger": trigger})
	if err != nil {
		return nil, err
	}
	var out Session
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel cancels a running session.
func (c *SessionClient) Cancel(ctx context.Context, id string) (*Session, error) {
	data, err := c.client.doJSON(ctx, "POST", fmt.Sprintf("/sessions/%s/cancel", id), nil)
	if err != nil {
		return nil, err
	}
	var out Session
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TraceClient operates on traces.
type TraceClient struct {
	client *Client
}

// List returns traces.
func (c *TraceClient) List(ctx context.Context, domain, sessionID string, limit, offset int) (*ListEnvelope[TraceSummary], error) {
	path := "/traces" + queryString(map[string]any{"domain": domain, "session": sessionID, "limit": limit, "offset": offset})
	data, err := c.client.doJSON(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out ListEnvelope[TraceSummary]
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a single trace by ID.
func (c *TraceClient) Get(ctx context.Context, traceID string) (*Trace, error) {
	data, err := c.client.doJSON(ctx, "GET", fmt.Sprintf("/traces/%s", traceID), nil)
	if err != nil {
		return nil, err
	}
	var out Trace
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApprovalClient operates on approvals.
type ApprovalClient struct {
	client *Client
}

// List returns approvals.
func (c *ApprovalClient) List(ctx context.Context, domain, sessionID, status string) (*ListEnvelope[Approval], error) {
	path := "/approvals" + queryString(map[string]any{"domain": domain, "session": sessionID, "status": status})
	data, err := c.client.doJSON(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out ListEnvelope[Approval]
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Approve approves a pending approval.
func (c *ApprovalClient) Approve(ctx context.Context, id string) (*Approval, error) {
	data, err := c.client.doJSON(ctx, "POST", fmt.Sprintf("/approvals/%s/approve", id), nil)
	if err != nil {
		return nil, err
	}
	var out Approval
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Reject rejects a pending approval.
func (c *ApprovalClient) Reject(ctx context.Context, id string) (*Approval, error) {
	data, err := c.client.doJSON(ctx, "POST", fmt.Sprintf("/approvals/%s/reject", id), nil)
	if err != nil {
		return nil, err
	}
	var out Approval
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EvalClient operates on eval sets and runs.
type EvalClient struct {
	client *Client
}

// ListSets returns all eval sets.
func (c *EvalClient) ListSets(ctx context.Context) (*ListEnvelope[EvalSetSummary], error) {
	data, err := c.client.doJSON(ctx, "GET", "/evals", nil)
	if err != nil {
		return nil, err
	}
	var out ListEnvelope[EvalSetSummary]
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSet returns an eval set by ID.
func (c *EvalClient) GetSet(ctx context.Context, id string) (*EvalSet, error) {
	data, err := c.client.doJSON(ctx, "GET", fmt.Sprintf("/evals/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var out EvalSet
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSet creates a new eval set.
func (c *EvalClient) CreateSet(ctx context.Context, setID, domainID, description string, cases []EvalCase) (*EvalSet, error) {
	payload := map[string]any{"set_id": setID, "domain_id": domainID, "description": description, "cases": cases}
	data, err := c.client.doJSON(ctx, "POST", "/evals", payload)
	if err != nil {
		return nil, err
	}
	var out EvalSet
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RunSet starts an eval run.
func (c *EvalClient) RunSet(ctx context.Context, id string) (map[string]any, error) {
	data, err := c.client.doJSON(ctx, "POST", fmt.Sprintf("/evals/%s/run", id), nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
