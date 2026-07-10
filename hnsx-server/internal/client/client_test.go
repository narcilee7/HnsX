package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "SESSION_NOT_FOUND",
			"message": "session x not found",
		})
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	_, err := c.GetSession("x")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "SESSION_NOT_FOUND" {
		t.Fatalf("expected SESSION_NOT_FOUND, got %s", apiErr.Code)
	}
}

func TestCancelSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/sessions/foo/cancel") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "foo", "state": "cancelled"})
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	s, err := c.CancelSession("foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "foo" || s.State != "cancelled" {
		t.Fatalf("unexpected session: %+v", s)
	}
}

func TestSessionEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flush")
		}
		_, _ = w.Write([]byte("event: state\ndata: {\"state\":\"running\"}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, errCh, err := c.SessionEvents(ctx, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var names []string
	for evt := range events {
		names = append(names, evt.Name)
	}
	if len(errCh) > 0 {
		t.Fatalf("unexpected stream error: %v", <-errCh)
	}

	if len(names) != 2 || names[0] != "state" || names[1] != "done" {
		t.Fatalf("unexpected events: %v", names)
	}
}
