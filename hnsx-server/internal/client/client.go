// Package client is a thin client for the HnsX server. It speaks Connect-RPC
// for the control-plane services defined in proto/hnsx/v1/control_plane.proto
// and falls back to HTTP/REST for operations that are not yet exposed over
// Connect (session trigger/cancel/rerun and SSE events).
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1/v1connect"
)

// DefaultBaseURL is the local hnsx-server HTTP endpoint.
const DefaultBaseURL = "http://127.0.0.1:50051"

// APIError mirrors the canonical server error envelope.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// DomainListItem is the API list view of a registered domain.
type DomainListItem = viewmodel.DomainListItem

// Domain is the API detail view of a registered domain.
type Domain = viewmodel.DomainDetail

// SessionListItem is the API list view of a session.
type SessionListItem = viewmodel.SessionListItem

// Session is the API detail view of a session.
type Session = viewmodel.SessionDetail

// Event is one Server-Sent Event received from /sessions/:id/events.
type Event struct {
	Name    string
	Payload []byte
}

// Client talks to the HnsX server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	AuthToken  string

	domainClient    v1connect.DomainRegistryServiceClient
	sessionClient   v1connect.SessionSchedulerServiceClient
	evalClient      v1connect.EvalServiceClient
	runtimeClient   v1connect.RuntimeDiscoveryServiceClient
	telemetryClient v1connect.TelemetryServiceClient
}

// New constructs a Client using HNSX_SERVER_URL or the default local endpoint.
func New() *Client {
	base := os.Getenv("HNSX_SERVER_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	c := NewWithBaseURL(base)
	if token := os.Getenv("HNSX_TOKEN"); token != "" {
		c.AuthToken = token
	}
	return c
}

// NewWithBaseURL constructs a Client for an explicit server URL.
func NewWithBaseURL(baseURL string) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	c := &Client{BaseURL: baseURL, HTTPClient: httpClient}
	auth := newAuthInterceptor(c)
	c.domainClient = v1connect.NewDomainRegistryServiceClient(httpClient, baseURL, connect.WithHTTPGet(), connect.WithInterceptors(auth))
	c.sessionClient = v1connect.NewSessionSchedulerServiceClient(httpClient, baseURL, connect.WithHTTPGet(), connect.WithInterceptors(auth))
	c.evalClient = v1connect.NewEvalServiceClient(httpClient, baseURL, connect.WithHTTPGet(), connect.WithInterceptors(auth))
	c.runtimeClient = v1connect.NewRuntimeDiscoveryServiceClient(httpClient, baseURL, connect.WithHTTPGet(), connect.WithInterceptors(auth))
	c.telemetryClient = v1connect.NewTelemetryServiceClient(httpClient, baseURL, connect.WithHTTPGet(), connect.WithInterceptors(auth))
	return c
}

// newAuthInterceptor returns a Connect interceptor that attaches the current
// client's bearer token to every outgoing request.
func newAuthInterceptor(c *Client) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if c.AuthToken != "" {
				req.Header().Set("Authorization", "Bearer "+c.AuthToken)
			}
			return next(ctx, req)
		}
	}
}

// ListDomains returns every registered domain.
func (c *Client) ListDomains() ([]DomainListItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.domainClient.ListDomains(ctx, connect.NewRequest(&pb.ListDomainsRequest{}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	var out []DomainListItem
	for _, d := range resp.Msg.GetDomains() {
		out = append(out, DomainListItem{
			ID:          d.GetId(),
			Version:     d.GetVersion(),
			Description: d.GetDescription(),
			Status:      "active",
		})
	}
	return out, nil
}

// GetDomain returns a single domain by ID.
func (c *Client) GetDomain(id string) (*Domain, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.domainClient.GetDomain(ctx, connect.NewRequest(&pb.GetDomainRequest{Domain: &pb.DomainRef{Id: id}}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	return domainFromProto(resp.Msg.GetSpec())
}

// RegisterDomain uploads a DomainSpec and returns the registered domain.
func (c *Client) RegisterDomain(body io.Reader, contentType string) (*Domain, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var ds *domain.DomainSpec
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml") {
		ds, err = domain.Parse(data)
	} else {
		ds = new(domain.DomainSpec)
		err = json.Unmarshal(data, ds)
	}
	if err != nil {
		return nil, fmt.Errorf("parse domain: %w", err)
	}
	pbSpec, err := domain.ToProto(ds)
	if err != nil {
		return nil, fmt.Errorf("convert domain: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.domainClient.RegisterDomain(ctx, connect.NewRequest(&pb.RegisterDomainRequest{Spec: pbSpec}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	return c.GetDomain(resp.Msg.GetDomain().GetId())
}

// ListSessions returns every session.
func (c *Client) ListSessions() ([]SessionListItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.sessionClient.ListSessions(ctx, connect.NewRequest(&pb.ListSessionsRequest{}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	var out []SessionListItem
	for _, s := range resp.Msg.GetSessions() {
		out = append(out, sessionListItemFromProto(s))
	}
	return out, nil
}

// GetSession returns a single session by ID.
func (c *Client) GetSession(id string) (*Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.sessionClient.GetSession(ctx, connect.NewRequest(&pb.GetSessionRequest{SessionId: id}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	return sessionFromProto(resp.Msg.GetSession()), nil
}

// TriggerSession starts a new session for the given domain.
// This operation is not yet exposed over Connect; it uses the REST API.
func (c *Client) TriggerSession(domainID string, trigger map[string]any) (*Session, error) {
	payload := map[string]any{
		"domain_id": domainID,
		"trigger":   trigger,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.post("/api/v1/sessions", bytes.NewReader(b), "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// CancelSession cancels a session by ID.
func (c *Client) CancelSession(id string) (*Session, error) {
	resp, err := c.post("/api/v1/sessions/"+id+"/cancel", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// RerunSession reruns an existing session by ID.
func (c *Client) RerunSession(id string) (*Session, error) {
	resp, err := c.post("/api/v1/sessions/"+id+"/rerun", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SessionEvents opens the SSE stream for a session and returns a channel of
// events. The channel closes when the server sends a "done" event or when the
// context is cancelled. Callers should drain the channel.
func (c *Client) SessionEvents(ctx context.Context, id string) (<-chan Event, <-chan error, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/sessions/"+id+"/events", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if err := checkStatus(resp); err != nil {
		_ = resp.Body.Close()
		return nil, nil, err
	}

	events := make(chan Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errCh)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var current Event
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if current.Name != "" {
					select {
					case events <- current:
					case <-ctx.Done():
						errCh <- ctx.Err()
						return
					}
				}
				current = Event{}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				current.Name = strings.TrimPrefix(line, "event: ")
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				current.Payload = append(current.Payload, []byte(strings.TrimPrefix(line, "data: "))...)
				continue
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	return events, errCh, nil
}

// DomainRegistryClient returns the underlying Connect domain client for
// advanced callers.
func (c *Client) DomainRegistryClient() v1connect.DomainRegistryServiceClient { return c.domainClient }

// SessionSchedulerClient returns the underlying Connect session client.
func (c *Client) SessionSchedulerClient() v1connect.SessionSchedulerServiceClient {
	return c.sessionClient
}

// EvalClient returns the underlying Connect eval client.
func (c *Client) EvalClient() v1connect.EvalServiceClient { return c.evalClient }

func (c *Client) get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(req)
	return c.HTTPClient.Do(req)
}

func (c *Client) post(path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.setAuthHeader(req)
	return c.HTTPClient.Do(req)
}

func (c *Client) setAuthHeader(req *http.Request) {
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Code != "" {
			return &apiErr
		}
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}
	return nil
}

func mapConnectError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return &APIError{
			Code:    connectErr.Code().String(),
			Message: connectErr.Message(),
		}
	}
	if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
		return &APIError{Code: "TIMEOUT", Message: urlErr.Error()}
	}
	return err
}

func domainFromProto(pbSpec *pb.DomainSpec) (*Domain, error) {
	if pbSpec == nil {
		return nil, fmt.Errorf("nil domain spec")
	}
	ds, err := domain.FromProto(pbSpec)
	if err != nil {
		return nil, err
	}
	var harness any
	if ds.Harness.Agents != nil {
		harness = ds.Harness
	}
	return &Domain{
		ID:          ds.ID,
		Version:     ds.Version,
		Description: ds.Description,
		Harness:     harness,
		Spec:        ds,
		Status:      "active",
	}, nil
}

func sessionListItemFromProto(s *pb.SessionStatus) SessionListItem {
	started := time.UnixMilli(s.GetStartedAtMs())
	var completed *time.Time
	if ms := s.GetCompletedAtMs(); ms > 0 {
		t := time.UnixMilli(ms)
		completed = &t
	}
	return SessionListItem{
		ID:            s.GetSessionId(),
		DomainID:      s.GetDomainId(),
		DomainVersion: s.GetDomainVersion(),
		Orchestration: "",
		State:         s.GetState(),
		StartedAt:     started,
		CompletedAt:   completed,
	}
}

func sessionFromProto(s *pb.SessionStatus) *Session {
	if s == nil {
		return nil
	}
	started := time.UnixMilli(s.GetStartedAtMs())
	var completed *time.Time
	if ms := s.GetCompletedAtMs(); ms > 0 {
		t := time.UnixMilli(ms)
		completed = &t
	}
	var result map[string]any
	if s.GetResult() != "" {
		_ = json.Unmarshal([]byte(s.GetResult()), &result)
	}
	return &Session{
		ID:            s.GetSessionId(),
		DomainID:      s.GetDomainId(),
		DomainVersion: s.GetDomainVersion(),
		Orchestration: "",
		State:         s.GetState(),
		Trigger:       nil,
		StartedAt:     started,
		CompletedAt:   completed,
		Result:        result,
	}
}

// EvalSet is the API view of an evaluation set.
type EvalSet = viewmodel.EvalSet

// EvalCase is one test case inside an EvalSet.
type EvalCase = viewmodel.EvalCase

// EvalScorer defines how to score a case.
type EvalScorer = viewmodel.EvalScorer

// EvalRun is the API view of an evaluation run.
type EvalRun = viewmodel.EvalRun

// EvalCaseResult is the outcome of one EvalCase within a run.
type EvalCaseResult = viewmodel.EvalCaseResult

// ListEvalSets returns all eval sets.
func (c *Client) ListEvalSets() ([]EvalSet, error) {
	resp, err := c.get("/api/v1/evals")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Items []EvalSet `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return envelope.Items, nil
}

// CreateEvalSet creates a new eval set.
func (c *Client) CreateEvalSet(setID, domainID, description string, cases []EvalCase) (*EvalSet, error) {
	payload := map[string]any{
		"set_id":      setID,
		"domain_id":   domainID,
		"description": description,
		"cases":       cases,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.post("/api/v1/evals", bytes.NewReader(b), "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var set EvalSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	return &set, nil
}

// GetEvalSet returns an eval set by ID.
func (c *Client) GetEvalSet(id string) (*EvalSet, error) {
	resp, err := c.get("/api/v1/evals/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var set EvalSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	return &set, nil
}

// UpdateEvalSet updates an eval set.
func (c *Client) UpdateEvalSet(id, description string, cases []EvalCase) (*EvalSet, error) {
	payload := map[string]any{
		"description": description,
		"cases":       cases,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/api/v1/evals/"+id, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var set EvalSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	return &set, nil
}

// DeleteEvalSet removes an eval set by ID.
func (c *Client) DeleteEvalSet(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/v1/evals/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// RunEval starts a new eval run for the given set.
func (c *Client) RunEval(setID string) (*EvalRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.evalClient.RunEval(ctx, connect.NewRequest(&pb.RunEvalRequest{SetId: setID}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	return c.GetEvalRun(setID, resp.Msg.GetEvalRunId())
}

// ListEvalRuns returns all runs for an eval set.
func (c *Client) ListEvalRuns(setID string) ([]EvalRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.evalClient.ListEvalRuns(ctx, connect.NewRequest(&pb.ListEvalRunsRequest{SetId: setID}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	var out []EvalRun
	for _, r := range resp.Msg.GetResults() {
		out = append(out, *evalRunFromProto(r))
	}
	return out, nil
}

// GetEvalRun returns a single eval run.
func (c *Client) GetEvalRun(setID, runID string) (*EvalRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.evalClient.GetEvalRun(ctx, connect.NewRequest(&pb.GetEvalRunRequest{EvalRunId: runID}))
	if err != nil {
		return nil, mapConnectError(err)
	}
	return evalRunFromProto(resp.Msg.GetResult()), nil
}

func evalRunFromProto(r *pb.EvalRunResult) *EvalRun {
	if r == nil {
		return nil
	}
	cases := make([]EvalCaseResult, 0, len(r.GetCases()))
	for _, c := range r.GetCases() {
		var actual, details map[string]any
		_ = json.Unmarshal([]byte(c.GetActual()), &actual)
		_ = json.Unmarshal([]byte(c.GetDetails()), &details)
		cases = append(cases, EvalCaseResult{
			CaseID:    c.GetCaseId(),
			SessionID: c.GetSessionId(),
			Score:     c.GetScore(),
			Passed:    c.GetPassed(),
			Actual:    actual,
			Details:   details,
		})
	}
	return &EvalRun{
		ID:            r.GetEvalRunId(),
		EvalSetID:     r.GetSetId(),
		DomainID:      r.GetDomainId(),
		DomainVersion: "",
		Orchestration: "",
		State:         r.GetState(),
		Score:         r.GetScore(),
		TotalCases:    int(r.GetTotal()),
		PassedCases:   int(r.GetPassed()),
		TotalCostUSD:  r.GetTotalCostUsd(),
		DurationMs:    r.GetDurationMs(),
		BaselineRunID: r.GetBaselineRunId(),
		Cases:         cases,
	}
}
