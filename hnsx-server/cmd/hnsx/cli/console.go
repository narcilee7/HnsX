package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// newConsoleCmd starts the HnsX Web Console (hnsx-console/) and opens the
// browser. It supports two modes:
//
//	--dev      run `pnpm dev` (Vite dev server, hot reload)
//	default    run `pnpm preview` against an existing build, or `pnpm build`
//	            first if dist/ is missing
func newConsoleCmd(cfg *Config) *cobra.Command {
	var (
		dev    bool
		port   int
		noOpen bool
		host   string
	)
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Start the HnsX Web Console (Vite) and open it in your browser",
		Long: `Launches the React Web Console that lives at hnsx-console/ in this
repo, then opens it in your default browser.

By default uses the production preview server (pnpm preview) on port 5173;
use --dev for the hot-reloading dev server.

The command blocks until Ctrl-C.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			consoleDir := filepath.Join(cfg.RepoRoot, "hnsx-console")
			if _, err := os.Stat(consoleDir); err != nil {
				return fmt.Errorf("console dir not found: %s (run from HnsX repo root)", consoleDir)
			}
			if _, err := os.Stat(filepath.Join(consoleDir, "node_modules")); err != nil {
				return fmt.Errorf("console dependencies not installed; run `pnpm install` in %s", consoleDir)
			}
			if !dev {
				if _, err := os.Stat(filepath.Join(consoleDir, "dist")); err != nil {
					out := NewOutput(cfg.Output)
					out.Line("→ building console (no dist/ found) ...")
					if err := runPnpm(consoleDir, "build"); err != nil {
						return fmt.Errorf("build console: %w", err)
					}
				}
			}

			addr := fmt.Sprintf("%s:%d", host, port)
			if err := checkPortFree(addr); err != nil {
				return fmt.Errorf("port %d unavailable: %w", port, err)
			}

			script := "preview"
			if dev {
				script = "dev"
			}
			previewArgs := []string{script}
			if dev {
				previewArgs = append(previewArgs, "--host", host, "--port", fmt.Sprintf("%d", port))
			} else {
				previewArgs = append(previewArgs, "--port", fmt.Sprintf("%d", port))
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			c := exec.CommandContext(ctx, "pnpm", previewArgs...)
			c.Dir = consoleDir
			c.Env = append(os.Environ(), "BROWSER=none") // we open ourselves
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Start(); err != nil {
				return fmt.Errorf("start console: %w", err)
			}

			url := fmt.Sprintf("http://%s:%d", host, port)
			NewOutput(cfg.Output).Line("✓ console starting at %s", url)

			// Wait for /  to respond (up to 30s), then open the browser.
			if err := waitHTTP(url, 30*time.Second); err != nil {
				c.Process.Kill()
				return fmt.Errorf("console did not become reachable: %w", err)
			}
			if !noOpen {
				if err := openBrowser(url); err != nil {
					NewOutput(cfg.Output).Line("⚠ could not auto-open browser: %v", err)
				}
			}

			// Block until child exits or Ctrl-C.
			done := make(chan error, 1)
			go func() { done <- c.Wait() }()
			select {
			case <-ctx.Done():
				_ = c.Process.Kill()
				return nil
			case err := <-done:
				return err
			}
		},
	}
	cmd.Flags().BoolVar(&dev, "dev", false, "run Vite dev server (hot reload)")
	cmd.Flags().IntVar(&port, "port", 5173, "port to serve on")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open browser automatically")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host to bind")
	return cmd
}

// runPnpm runs `pnpm <args>` in dir with stdout streaming.
func runPnpm(dir string, args ...string) error {
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// checkPortFree returns nil if addr is bindable.
func checkPortFree(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	_ = l.Close()
	return nil
}

// waitHTTP polls url until a 2xx/3xx response or timeout.
func waitHTTP(url string, d time.Duration) error {
	deadline := time.Now().Add(d)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", d)
}

// openBrowser opens url in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// newUpdateCmd was a placeholder; the real implementation lives in
// update.go (v0.8). The constructor is registered by root.go.
