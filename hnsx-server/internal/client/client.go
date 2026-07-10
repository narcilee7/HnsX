// Package client is a thin HTTP client for the HnsX server API. It is used by
// the CLI remote subcommands and can be reused by other Go consumers.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// DefaultBaseURL is the local hnsx-server HTTP endpoint.
const DefaultBaseURL = "http://127.0.0.1:50051"

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
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}
	return nil
}
