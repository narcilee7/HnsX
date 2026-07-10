package spec

import (
	"os"
	"testing"
)

func TestToProtoRoundTrip(t *testing.T) {
	for _, path := range []string{
		"../../../example-domains/customer-service/domain.yaml",
		"../../../example-domains/workflow-demo/domain.yaml",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			original, err := Parse(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			pb, err := ToProto(original)
			if err != nil {
				t.Fatalf("ToProto: %v", err)
			}
			back, err := FromProto(pb)
			if err != nil {
				t.Fatalf("FromProto: %v", err)
			}
			if back.ID != original.ID {
				t.Errorf("id mismatch: %q vs %q", back.ID, original.ID)
			}
			if back.Version != original.Version {
				t.Errorf("version mismatch: %q vs %q", back.Version, original.Version)
			}
			if len(back.Harness.Agents) != len(original.Harness.Agents) {
				t.Errorf("agent count mismatch: %d vs %d", len(back.Harness.Agents), len(original.Harness.Agents))
			}
			if len(back.Harness.Tools) != len(original.Harness.Tools) {
				t.Errorf("tool count mismatch: %d vs %d", len(back.Harness.Tools), len(original.Harness.Tools))
			}
			if back.Harness.Session.Mode != original.Harness.Session.Mode {
				t.Errorf("session mode mismatch: %q vs %q", back.Harness.Session.Mode, original.Harness.Session.Mode)
			}
		})
	}
}

func TestToProtoNil(t *testing.T) {
	pb, err := ToProto(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pb != nil {
		t.Fatalf("expected nil, got %v", pb)
	}
	back, err := FromProto(nil)
	if err != nil {
		t.Fatal(err)
	}
	if back != nil {
		t.Fatalf("expected nil, got %v", back)
	}
}
