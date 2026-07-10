// Package client is a thin HTTP client for the HnsX server API. It is used by
// the CLI remote subcommands and can be reused by other Go consumers.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
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

// Client talks to the HnsX REST API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListDomains returns every registered domain.
func (c *Client) ListDomains() ([]DomainListItem, error) {
	resp, err := c.get("/api/v1/domains")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Items []DomainListItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return envelope.Items, nil
}

// GetDomain returns a single domain by ID.
func (c *Client) GetDomain(id string) (*Domain, error) {
	resp, err := c.get("/api/v1/domains/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var d Domain
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// RegisterDomain uploads a DomainSpec and returns the registered domain.
func (c *Client) RegisterDomain(body io.Reader, contentType string) (*Domain, error) {
	resp, err := c.post("/api/v1/domains", body, contentType)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var d Domain
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ListSessions returns every session.
func (c *Client) ListSessions() ([]SessionListItem, error) {
	resp, err := c.get("/api/v1/sessions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Items []SessionListItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return envelope.Items, nil
}

// GetSession returns a single session by ID.
func (c *Client) GetSession(id string) (*Session, error) {
	resp, err := c.get("/api/v1/sessions/" + id)
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

// TriggerSession starts a new session for the given domain.
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
