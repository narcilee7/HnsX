package app

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

func TestState_TenantIsolation(t *testing.T) {
	s := NewState()
	t1 := tenant.ID("tenant-1")
	t2 := tenant.ID("tenant-2")

	s.RegisterDomain(t1, &RegisteredDomain{ID: "d1", Version: "v1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	s.RegisterDomain(t2, &RegisteredDomain{ID: "d1", Version: "v2", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})

	if len(s.ListDomains(t1)) != 1 {
		t.Fatalf("tenant-1 domain count = %d, want 1", len(s.ListDomains(t1)))
	}
	if len(s.ListDomains(t2)) != 1 {
		t.Fatalf("tenant-2 domain count = %d, want 1", len(s.ListDomains(t2)))
	}

	d1, _ := s.LookupDomain(t1, "d1")
	if d1.Version != "v1" {
		t.Fatalf("tenant-1 domain version = %q, want v1", d1.Version)
	}
	d2, _ := s.LookupDomain(t2, "d1")
	if d2.Version != "v2" {
		t.Fatalf("tenant-2 domain version = %q, want v2", d2.Version)
	}
}

func TestState_SessionTenantIsolation(t *testing.T) {
	s := NewState()
	t1 := tenant.ID("tenant-a")
	t2 := tenant.ID("tenant-b")

	s.RegisterSession(t1, &RegisteredSession{ID: "s1", State: "pending", StartedAt: time.Now().UTC()})
	s.RegisterSession(t2, &RegisteredSession{ID: "s2", State: "pending", StartedAt: time.Now().UTC()})

	if len(s.ListSessions(t1)) != 1 {
		t.Fatalf("tenant-a session count = %d, want 1", len(s.ListSessions(t1)))
	}
	if len(s.ListSessions(t2)) != 1 {
		t.Fatalf("tenant-b session count = %d, want 1", len(s.ListSessions(t2)))
	}

	if _, ok := s.LookupSession(t1, "s2"); ok {
		t.Fatal("tenant-a should not see tenant-b session")
	}
}

func TestState_DefaultTenantEmpty(t *testing.T) {
	s := NewState()
	if len(s.ListDomains(tenant.DefaultID)) != 0 {
		t.Fatalf("default tenant should start empty")
	}
	if len(s.ListSessions(tenant.DefaultID)) != 0 {
		t.Fatalf("default tenant sessions should start empty")
	}
}
