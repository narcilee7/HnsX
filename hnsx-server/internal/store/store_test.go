package store

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

func TestInMemoryBackend(t *testing.T) {
	b := NewInMemoryBackend()

	if got, err := b.Get(NamespaceContext, "foo"); err != nil || got != "" {
		t.Fatalf("missing key: got %q err %v", got, err)
	}

	if err := b.Set(NamespaceContext, "foo", "bar"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, err := b.Get(NamespaceContext, "foo"); err != nil || got != "bar" {
		t.Fatalf("get after set: got %q err %v", got, err)
	}

	if got, err := b.Get(NamespaceKnowledge, "foo"); err != nil || got != "" {
		t.Fatalf("namespace isolation failed: got %q err %v", got, err)
	}

	if err := b.Delete(NamespaceContext, "foo"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, err := b.Get(NamespaceContext, "foo"); err != nil || got != "" {
		t.Fatalf("after delete: got %q err %v", got, err)
	}
}

func TestInMemoryBackendInvalidKey(t *testing.T) {
	b := NewInMemoryBackend()
	if err := b.Set(NamespaceContext, "", "x"); err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestNewBackendFromSpecDefaults(t *testing.T) {
	b, err := NewBackendFromSpec(nil)
	if err != nil {
		t.Fatalf("nil config: %v", err)
	}
	if b == nil {
		t.Fatal("nil backend")
	}
}

func TestNewBackendFromSpecUnsupported(t *testing.T) {
	cfg := &spec.StoreConfig{
		Context: spec.StoreNamespaceConfig{Backend: "redis"},
	}
	if _, err := NewBackendFromSpec(cfg); err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}
