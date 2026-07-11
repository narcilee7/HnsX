package cli

import (
	"bytes"
	"testing"
)

func TestNewRootCmd_NoArgs_NonTTY(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{})

	// In test environments stdout is not a terminal, so the root command should
	// fall back to printing help rather than launching the TUI.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with no args: %v", err)
	}
}

func TestNewRootCmd_NoTuiFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--no-tui"})

	// --no-tui must be a valid persistent flag and should also result in help.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with --no-tui: %v", err)
	}
}

func TestNewRootCmd_NoTuiEnv(t *testing.T) {
	t.Setenv("HNSX_NO_TUI", "true")
	cfg := Default()
	if !cfg.NoTui {
		t.Fatal("HNSX_NO_TUI=true should set cfg.NoTui")
	}
}

func TestNewRootCmd_SubcommandStillWorks(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"version"})

	// The version command writes directly to os.Stdout, so we only verify that
	// the subcommand is still reachable after the TUI entry changes.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with version: %v", err)
	}
}
