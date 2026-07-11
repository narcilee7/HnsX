package worker

import (
	"testing"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func TestCapabilitiesFromInfo(t *testing.T) {
	info := &pb.WorkerInfo{
		Capacity: &pb.ResourceCapacity{
			Providers:       []string{"anthropic", "openai"},
			Models:          []string{"claude-haiku-4-5"},
			SandboxRuntimes: []string{"process"},
		},
		Labels: map[string]string{"zone": "a"},
	}
	got := CapabilitiesFromInfo(info)
	want := map[string]bool{
		"provider:anthropic":     true,
		"provider:openai":        true,
		"adapter:anthropic":      true,
		"adapter:openai":         true,
		"model:claude-haiku-4-5": true,
		"sandbox:process":        true,
		"label:zone:a":           true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d capabilities, want %d: %v", len(got), len(want), got)
	}
	for _, c := range got {
		if !want[c] {
			t.Fatalf("unexpected capability: %s", c)
		}
	}
}

func TestCapabilitiesFromInfo_Nil(t *testing.T) {
	if got := CapabilitiesFromInfo(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
