package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPluginDir(t *testing.T) {
	t.Setenv("HNSX_PLUGIN_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "hnsx", "plugins")
	if got := PluginDir(); got != want {
		t.Fatalf("PluginDir() = %q, want %q", got, want)
	}
}

func TestPluginDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	want := "/xdg/hnsx/plugins"
	if got := PluginDir(); got != want {
		t.Fatalf("PluginDir() = %q, want %q", got, want)
	}
}

func TestPluginDir_Override(t *testing.T) {
	t.Setenv("HNSX_PLUGIN_DIR", "/plugins")
	if got := PluginDir(); got != "/plugins" {
		t.Fatalf("PluginDir() = %q, want /plugins", got)
	}
}

func TestTryPluginExec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HNSX_PLUGIN_DIR", dir)

	script := filepath.Join(dir, pluginPrefix+"hello")
	if runtime.GOOS == "windows" {
		script += ".bat"
		os.WriteFile(script, []byte("@echo off\necho hello-plugin\n"), 0o755)
	} else {
		os.WriteFile(script, []byte("#!/bin/sh\necho hello-plugin\n"), 0o755)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"hnsx", "hello"}

	ran, err := TryPluginExec()
	if !ran {
		t.Fatal("expected plugin to run")
	}
	if err != nil {
		t.Fatalf("plugin error: %v", err)
	}
}

func TestTryPluginExec_KnownCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"hnsx", "version"}

	ran, _ := TryPluginExec()
	if ran {
		t.Fatal("expected version to be reserved")
	}
}

func TestTryPluginExec_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HNSX_PLUGIN_DIR", dir)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"hnsx", "nonexistent"}

	ran, err := TryPluginExec()
	if ran {
		t.Fatal("expected no plugin to run")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGuessPluginName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://example.com/hnsx-foo", "foo"},
		{"/path/to/hnsx-foo", "foo"},
		{"hnsx-bar-darwin-arm64.tar.gz", "bar"},
		{"hnsx-baz-v1.2.3", "baz"},
		{"plain-name", "plain-name"},
	}
	for _, c := range cases {
		if got := guessPluginName(c.in); got != c.want {
			t.Errorf("guessPluginName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsLocalPath(t *testing.T) {
	f := filepath.Join(t.TempDir(), "plugin")
	os.WriteFile(f, []byte("x"), 0o644)
	if !isLocalPath(f) {
		t.Fatal("expected local path")
	}
	if isLocalPath("https://example.com/plugin") {
		t.Fatal("expected URL")
	}
}

func TestPluginListCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HNSX_PLUGIN_DIR", dir)
	os.WriteFile(filepath.Join(dir, pluginPrefix+"foo"), []byte(""), 0o755)
	os.WriteFile(filepath.Join(dir, pluginPrefix+"bar"), []byte(""), 0o755)

	listCmd := newPluginListCmd()
	out, err := executeCommand(listCmd)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Fatalf("expected foo and bar in output, got %q", out)
	}
}

func TestPluginInstallUninstallCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HNSX_PLUGIN_DIR", dir)

	src := filepath.Join(t.TempDir(), "hnsx-demo")
	os.WriteFile(src, []byte("#!/bin/sh\necho demo"), 0o755)

	installCmd := newPluginInstallCmd()
	if _, err := executeCommand(installCmd, src); err != nil {
		t.Fatalf("install: %v", err)
	}

	want := filepath.Join(dir, pluginPrefix+"demo")
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("plugin not installed: %v", err)
	}

	uninstallCmd := newPluginUninstallCmd()
	if _, err := executeCommand(uninstallCmd, "demo"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("plugin still installed after uninstall")
	}
}

func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	cmd.SetArgs(args)
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	return buf.String(), err
}
