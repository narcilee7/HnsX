package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const pluginPrefix = "hnsx-"

// PluginDir returns the directory where external hnsx plugins are installed.
// It respects XDG_CONFIG_HOME and falls back to ~/.config/hnsx/plugins.
func PluginDir() string {
	if d := os.Getenv("HNSX_PLUGIN_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "hnsx", "plugins")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "hnsx", "plugins")
	}
	return ""
}

// TryPluginExec attempts to execute an external plugin when the first
// positional argument is not a built-in command. It returns true if a plugin
// was found and executed. On plugin-not-found it returns (false, nil) so the
// caller can fall back to normal cobra handling.
func TryPluginExec() (bool, error) {
	if len(os.Args) < 2 {
		return false, nil
	}
	first := os.Args[1]
	if first == "" || strings.HasPrefix(first, "-") {
		return false, nil
	}
	if isReservedCommand(first) {
		return false, nil
	}

	dir := PluginDir()
	if dir == "" {
		return false, nil
	}

	pluginName := pluginPrefix + first
	candidates := []string{
		filepath.Join(dir, pluginName),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(dir, pluginName+".exe"),
			filepath.Join(dir, pluginName+".bat"),
			filepath.Join(dir, pluginName+".cmd"),
		)
	}

	var pluginPath string
	for _, p := range candidates {
		if isExecutable(p) {
			pluginPath = p
			break
		}
	}
	if pluginPath == "" {
		return false, nil
	}

	cmd := exec.Command(pluginPath, os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = pluginEnv(pluginName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return true, err
	}
	return true, nil
}

// isReservedCommand lists top-level commands that should never be shadowed by
// a plugin. It is intentionally conservative; aliases are covered by cobra.
func isReservedCommand(name string) bool {
	switch name {
	case "help", "version", "--help", "-h", "--version", "-v":
		return true
	}
	return false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func pluginEnv(pluginName string) []string {
	base := os.Environ()
	// Let the plugin know it is being invoked as an hnsx plugin and preserve
	// the original command name for friendly error messages.
	base = append(base,
		"HNSX_PLUGIN=1",
		"HNSX_PLUGIN_NAME="+pluginName,
	)
	return base
}

// newPluginCmd implements `hnsx plugin list|install|uninstall|path`.
func newPluginCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage hnsx external plugins",
		Long: fmt.Sprintf(`Manage external hnsx plugins.

Plugins are executables named %s<command> located in:

    %s

When hnsx receives an unknown top-level command, it looks for a matching
plugin executable and invokes it with the remaining arguments.`, pluginPrefix, PluginDir()),
	}
	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginUninstallCmd())
	cmd.AddCommand(newPluginPathCmd())
	return cmd
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := PluginDir()
			if dir == "" {
				return fmt.Errorf("could not determine plugin directory")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintln(cmd.OutOrStdout(), "no plugins installed")
					return nil
				}
				return err
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if runtime.GOOS == "windows" {
					name = strings.TrimSuffix(name, filepath.Ext(name))
				}
				if !strings.HasPrefix(name, pluginPrefix) {
					continue
				}
				if !isExecutable(filepath.Join(dir, e.Name())) {
					continue
				}
				names = append(names, strings.TrimPrefix(name, pluginPrefix))
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no plugins installed")
				return nil
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		},
	}
}

func newPluginInstallCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "install <url|path>",
		Short: "Install a plugin from a URL or local path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			if name == "" {
				name = guessPluginName(source)
			}
			if name == "" {
				return fmt.Errorf("could not determine plugin name; use --name")
			}
			if strings.Contains(name, "/") || strings.Contains(name, string(filepath.Separator)) {
				return fmt.Errorf("invalid plugin name %q", name)
			}

			dir := PluginDir()
			if dir == "" {
				return fmt.Errorf("could not determine plugin directory")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}

			data, err := fetchPluginBinary(source)
			if err != nil {
				return fmt.Errorf("fetch plugin: %w", err)
			}

			target := filepath.Join(dir, pluginPrefix+name)
			if runtime.GOOS == "windows" {
				target += ".exe"
			}
			if err := os.WriteFile(target, data, 0o755); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ installed %s\n", target)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "plugin command name (default: derived from URL/path)")
	return cmd
}

func newPluginUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := PluginDir()
			if dir == "" {
				return fmt.Errorf("could not determine plugin directory")
			}
			name := args[0]
			target := filepath.Join(dir, pluginPrefix+name)
			candidates := []string{target}
			if runtime.GOOS == "windows" {
				candidates = append(candidates, target+".exe", target+".bat", target+".cmd")
			}
			removed := false
			for _, p := range candidates {
				if _, err := os.Stat(p); err == nil {
					if err := os.Remove(p); err != nil {
						return err
					}
					removed = true
				}
			}
			if !removed {
				return fmt.Errorf("plugin %q is not installed", name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ uninstalled %s\n", name)
			return nil
		},
	}
}

func newPluginPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the plugin directory path",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), PluginDir())
			return nil
		},
	}
}

func guessPluginName(source string) string {
	base := filepath.Base(source)
	// Strip any query/fragment from a URL first.
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	// Strip common archive/executable suffixes.
	for _, suffix := range []string{".tar.gz", ".tgz", ".zip", ".gz", ".exe"} {
		base = strings.TrimSuffix(base, suffix)
	}
	// If the basename starts with the plugin prefix, use the rest.
	if strings.HasPrefix(base, pluginPrefix) {
		rest := strings.TrimPrefix(base, pluginPrefix)
		// Common release artifacts: hnsx-foo-<os>-<arch> or hnsx-foo-v1.2.3.
		// The command is the first segment after the prefix.
		if i := strings.Index(rest, "-"); i > 0 {
			return rest[:i]
		}
		return rest
	}
	return base
}

func fetchPluginBinary(source string) ([]byte, error) {
	if isLocalPath(source) {
		return os.ReadFile(source)
	}
	resp, err := http.Get(source)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func isLocalPath(s string) bool {
	if strings.Contains(s, "://") {
		return false
	}
	_, err := os.Stat(s)
	return err == nil
}
