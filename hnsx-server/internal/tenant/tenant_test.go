package tenant

import (
	"context"
	"net/http"
	"testing"
)

func TestFromContext_Default(t *testing.T) {
	if got := FromContext(context.Background()); got != DefaultID {
		t.Fatalf("FromContext() = %q, want %q", got, DefaultID)
	}
}

func TestNewContext(t *testing.T) {
	ctx := NewContext(context.Background(), ID("tenant-abc"))
	if got := FromContext(ctx); got != "tenant-abc" {
		t.Fatalf("FromContext() = %q, want tenant-abc", got)
	}
}

func TestMiddleware(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != "tenant-header" {
			t.Fatalf("tenant from header = %q, want tenant-header", got)
		}
	}))
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderName, "tenant-header")
	handler.ServeHTTP(nil, req)
}

func TestMiddleware_Default(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != DefaultID {
			t.Fatalf("tenant default = %q, want %q", got, DefaultID)
		}
	}))
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(nil, req)
}
