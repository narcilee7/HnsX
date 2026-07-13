package app

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func TestState_AttachBroadcaster(t *testing.T) {
	s := NewState()
	bc1 := s.AttachBroadcaster("sess-1")
	if bc1 == nil {
		t.Fatal("expected non-nil broadcaster")
	}
	bc2 := s.AttachBroadcaster("sess-1")
	if bc1 != bc2 {
		t.Fatal("expected same broadcaster for the same session")
	}
}

func TestState_DetachBroadcaster(t *testing.T) {
	s := NewState()
	bc := s.AttachBroadcaster("sess-1")
	s.DetachBroadcaster("sess-1")

	if _, ok := s.Broadcaster("sess-1"); ok {
		t.Fatal("broadcaster should have been removed")
	}

	ch, unsub := bc.Subscribe()
	defer unsub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("detached broadcaster should be closed")
		}
	default:
		// closed channels return immediately with zero value; either is fine.
	}
}

func TestState_PublishObservation(t *testing.T) {
	s := NewState()
	bc := s.AttachBroadcaster("sess-1")
	ch, unsub := bc.Subscribe()
	defer unsub()

	obs := domain.Observation{Kind: "state", SessionID: "sess-1", Payload: map[string]any{"state": "running"}}
	if !s.PublishObservation("sess-1", obs) {
		t.Fatal("expected publish to succeed")
	}

	select {
	case got := <-ch:
		if got.Kind != "state" {
			t.Fatalf("observation kind = %q, want state", got.Kind)
		}
	default:
		t.Fatal("expected observation on subscriber channel")
	}
}

func TestState_BroadcasterLookupMissing(t *testing.T) {
	s := NewState()
	if _, ok := s.Broadcaster("missing"); ok {
		t.Fatal("missing broadcaster should not be found")
	}
}
