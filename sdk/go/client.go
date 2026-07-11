// Package hnsx provides a Go SDK for the HnsX REST API.
package hnsx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the entry point for the HnsX Go SDK.
type Client struct {
	baseURL string
	http    *http.Client
	headers map[string]string

	Domains    *DomainClient
	Sessions   *SessionClient
	Traces     *TraceClient
	Approvals  *ApprovalClient
	Evals      *EvalClient
}

// NewClient creates a Client for the given server base URL.
func NewClient(baseURL string) *Client {
	return NewClientWithHTTP(baseURL, &http.Client{Timeout: 30 * time.Second})
}

// NewClientWithHTTP creates a Client with a custom http.Client.
func NewClientWithHTTP(baseURL string, httpClient *http.Client) *Client {
	c := &Client{
		baseURL: baseURL,
		http:    httpClient,
		headers: make(map[string]string),
	}
	c.Domains = &DomainClient{client: c}
	c.Sessions = &SessionClient{client: c}
	c.Traces = &TraceClient{client: c}
	c.Approvals = &ApprovalClient{client: c}
	c.Evals = &EvalClient{client: c}
	return c
}

// SetHeader adds a default header sent with every request.
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, extraHeaders map[string]string) (*http.Response, error) {
	u, err := url.Parse(c.baseURL + "/api/v1" + path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	resp, err := c.request(ctx, method, path, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeResponse(resp)
}

func decodeResponse(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var env struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details map[string]any `json:"details"`
		}
		if err := json.Unmarshal(data, &env); err == nil && env.Code != "" {
			return nil, &APIError{Code: env.Code, Message: env.Message, Status: resp.StatusCode, Details: env.Details}
		}
		return nil, &APIError{Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(data), Status: resp.StatusCode}
	}
	return data, nil
}

func queryString(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}
	q := url.Values{}
	for k, v := range params {
		switch val := v.(type) {
		case string:
			if val != "" {
				q.Set(k, val)
			}
		case int:
			q.Set(k, strconv.Itoa(val))
		case bool:
			q.Set(k, strconv.FormatBool(val))
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}
