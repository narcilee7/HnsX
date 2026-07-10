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

	"github.com/hnsx-io/hnsx/server/pkg/spec"
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

// DomainListItem mirrors the API list view.
type DomainListItem struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Domain mirrors the API detail view.
type Domain struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Harness     map[string]any `json:"harness"`
	Status      string         `json:"status"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// SessionListItem mirrors the API list view.
type SessionListItem struct {
	ID            string `json:"id"`
	DomainID      string `json:"domain_id"`
	DomainVersion string `json:"domain_version"`
	Orchestration string `json:"orchestration"`
	State         string `json:"state"`
	StartedAt     string `json:"started_at"`
	CompletedAt   string `json:"completed_at"`
}

// Session mirrors the API detail view.
type Session struct {
	ID            string         `json:"id"`
	DomainID      string         `json:"domain_id"`
	DomainVersion string         `json:"domain_version"`
	Orchestration string         `json:"orchestration"`
	State         string         `json:"state"`
	Trigger       map[string]any `json:"trigger"`
	StartedAt     string         `json:"started_at"`
	CompletedAt   string         `json:"completed_at"`
	Result        map[string]any `json:"result"`
}

// Event is one Server-Sent Event received from /sessions/:id/events.
type Event struct {
	Name    string
	Payload []byte
}

// Client talks to the HnsX server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client

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
	return NewWithBaseURL(base)
}

// NewWithBaseURL constructs a Client for an explicit server URL.
func NewWithBaseURL(baseURL string) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &Client{
		BaseURL:         baseURL,
		HTTPClient:      httpClient,
		domainClient:    v1connect.NewDomainRegistryServiceClient(httpClient, baseURL, connect.WithHTTPGet()),
		sessionClient:   v1connect.NewSessionSchedulerServiceClient(httpClient, baseURL, connect.WithHTTPGet()),
		evalClient:      v1connect.NewEvalServiceClient(httpClient, baseURL, connect.WithHTTPGet()),
		runtimeClient:   v1connect.NewRuntimeDiscoveryServiceClient(httpClient, baseURL, connect.WithHTTPGet()),
		telemetryClient: v1connect.NewTelemetryServiceClient(httpClient, baseURL, connect.WithHTTPGet()),
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
	var ds *spec.DomainSpec
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml") {
		ds, err = spec.Parse(data)
	} else {
		ds = new(spec.DomainSpec)
		err = json.Unmarshal(data, ds)
	}
	if err != nil {
		return nil, fmt.Errorf("parse domain: %w", err)
	}
	pbSpec, err := spec.ToProto(ds)
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
func (c *Client) SessionSchedulerClient() v1connect.SessionSchedulerServiceClient { return c.sessionClient }

// EvalClient returns the underlying Connect eval client.
func (c *Client) EvalClient() v1connect.EvalServiceClient { return c.evalClient }

func (c *Client) get(path string) (*http.Response, error) {
	return c.HTTPClient.Get(c.BaseURL + path)
}

func (c *Client) post(path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.HTTPClient.Do(req)
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
	ds, err := spec.FromProto(pbSpec)
	if err != nil {
		return nil, err
	}
	harness := map[string]any{}
	if ds.Harness.Agents != nil {
		b, _ := json.Marshal(ds.Harness)
		_ = json.Unmarshal(b, &harness)
	}
	return &Domain{
		ID:          ds.ID,
		Version:     ds.Version,
		Description: ds.Description,
		Harness:     harness,
		Status:      "active",
	}, nil
}

func sessionListItemFromProto(s *pb.SessionStatus) SessionListItem {
	return SessionListItem{
		ID:            s.GetSessionId(),
		DomainID:      s.GetDomainId(),
		DomainVersion: s.GetDomainVersion(),
		State:         s.GetState(),
	}
}

func sessionFromProto(s *pb.SessionStatus) *Session {
	if s == nil {
		return nil
	}
	var result map[string]any
	if s.GetResult() != "" {
		_ = json.Unmarshal([]byte(s.GetResult()), &result)
	}
	return &Session{
		ID:            s.GetSessionId(),
		DomainID:      s.GetDomainId(),
		DomainVersion: s.GetDomainVersion(),
		State:         s.GetState(),
		Result:        result,
	}
}
