package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1/v1connect"
)

type fakeDomainServer struct {
	v1connect.UnimplementedDomainRegistryServiceHandler
}

func (f *fakeDomainServer) GetDomain(ctx context.Context, req *connect.Request[pb.GetDomainRequest]) (*connect.Response[pb.GetDomainResponse], error) {
	if req.Msg.GetDomain().GetId() == "missing" {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("domain missing not found"))
	}
	return connect.NewResponse(&pb.GetDomainResponse{
		Spec: &pb.DomainSpec{
			Id:      req.Msg.GetDomain().GetId(),
			Version: "1.0.0",
		},
	}), nil
}

func TestConnectAPIErrorParsing(t *testing.T) {
	_, h := v1connect.NewDomainRegistryServiceHandler(
		&fakeDomainServer{},
		connect.WithInterceptors(),
	)
	mux := http.NewServeMux()
	mux.Handle("/hnsx.v1.DomainRegistryService/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	_, err := c.GetDomain("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != connect.CodeNotFound.String() {
		t.Fatalf("expected %s, got %s", connect.CodeNotFound.String(), apiErr.Code)
	}
}

func TestGetDomainConnect(t *testing.T) {
	_, h := v1connect.NewDomainRegistryServiceHandler(
		&fakeDomainServer{},
		connect.WithInterceptors(),
	)
	mux := http.NewServeMux()
	mux.Handle("/hnsx.v1.DomainRegistryService/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	d, err := c.GetDomain("cs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.ID != "cs" || d.Version != "1.0.0" {
		t.Fatalf("unexpected domain: %+v", d)
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
