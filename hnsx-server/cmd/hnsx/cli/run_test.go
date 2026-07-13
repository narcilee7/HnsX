package cli

import (
	"bytes"
	"context"
	"testing"
)

func TestRunCmdRemoved(t *testing.T) {
	cfg := &Config{Output: "human"}
	cmd := newRunCmd(cfg)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error because hnsx run is removed")
	}
	if !contains(err.Error(), "removed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
