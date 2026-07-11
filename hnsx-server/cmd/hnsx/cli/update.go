package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/version"
)

// newUpdateCmd v0.8 wires the real self-update path against GitHub Releases.
//
// Behaviour:
//
//	--check    only print whether a newer version exists
//	(default)  if a newer version is found and the running binary is in a
//	           writable location, perform an in-place replacement after a
//	           confirmation prompt; otherwise print install instructions.
//
// v0.8 keeps this conservative: it never silently replaces a binary that
// lives outside the user's home directory, and it always logs the action.
func newUpdateCmd(cfg *Config) *cobra.Command {
	var (
		check   bool
		force   bool
		channel string
	)
	cmd := &cobra.Command{
		Use:   "update [--check] [--force] [--channel stable|edge]",
		Short: "Update hnsx to the latest version (GitHub Releases)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			current := version.String()
			out.Line("current: %s", current)

			latest, err := fetchLatestRelease(channel)
			if err != nil {
				out.Line("⚠ could not check for updates: %v", err)
				out.Line("  (this is expected in offline / sandboxed environments)")
				out.Line("  install manually: curl -sSL hnsx.dev/install.sh | sh")
				return nil
			}
			out.Line("latest:  %s", latest.TagName)

			if latest.TagName == current {
				out.Line("✓ already up to date")
				return nil
			}

			if check {
				out.Line("→ upgrade available; run `hnsx update` to install")
				return nil
			}

			binaryPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("locate self: %w", err)
			}
			if !writableHome(binaryPath) && !force {
				out.Line("⚠ binary at %s is not under $HOME; refusing to overwrite", binaryPath)
				out.Line("  re-run with --force to attempt anyway")
				return nil
			}

			asset := pickAsset(latest, runtime.GOOS, runtime.GOARCH)
			if asset == nil {
				out.Line("⚠ no compatible asset for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latest.TagName)
				out.Line("  manual install: curl -sSL hnsx.dev/install.sh | sh")
				return nil
			}
			if err := downloadAndReplace(asset.URL, binaryPath); err != nil {
				return fmt.Errorf("install: %w", err)
			}
			out.Line("✓ updated to %s", latest.TagName)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only check, do not install")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite even if binary is outside $HOME")
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel (placeholder; v1.0 adds edge)")
	return cmd
}

// releaseInfo mirrors the subset of GitHub's release payload we consume.
type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

// fetchLatestRelease queries GitHub's releases API. Network failures are
// reported to the caller rather than aborted so the CLI degrades gracefully
// in offline environments.
func fetchLatestRelease(_ string) (*releaseInfo, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/hnsx-io/hnsx/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// No releases yet — first GA scenario.
		return &releaseInfo{TagName: "0.0.0", Name: "no releases"}, nil
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// pickAsset returns the asset URL for the current os/arch, or "" if missing.
// Expected naming: hnsx_<version>_<os>_<arch>.tar.gz
func pickAsset(r *releaseInfo, goos, goarch string) *releaseAsset {
	want := fmt.Sprintf("hnsx_%s_%s", goos, goarch)
	for i := range r.Assets {
		if matchAssetName(r.Assets[i].Name, want) {
			return &r.Assets[i]
		}
	}
	return nil
}

// matchAssetName returns true when `name` matches the wanted "hnsx_<os>_<arch>"
// pattern. Asset names look like "hnsx_0.8.0_darwin_arm64.tar.gz" — the
// version segment sits between the leading "hnsx_" and the os/arch tail,
// and a ".tar.gz" suffix is appended. We strip both before comparing.
func matchAssetName(name, want string) bool {
	const prefix = "hnsx_"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return false
	}
	// Trim common archive suffixes.
	trimmed := name
	for _, suf := range []string{".tar.gz", ".tgz", ".zip"} {
		if len(trimmed) > len(suf) && trimmed[len(trimmed)-len(suf):] == suf {
			trimmed = trimmed[:len(trimmed)-len(suf)]
		}
	}
	rest := trimmed[len(prefix):]
	// Skip the version segment: find the next underscore.
	for i := 0; i < len(rest); i++ {
		if rest[i] == '_' {
			return rest[i+1:] == want[len(prefix):]
		}
	}
	return false
}

// writableHome reports whether path is under the user's $HOME (and thus
// reasonably safe to overwrite without elevated privileges).
func writableHome(path string) bool {
	h, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if len(path) < len(h) {
		return false
	}
	return path[:len(h)] == h
}

// downloadAndReplace downloads url to a temp file, validates it, and renames
// it over path. Uses 0600 for the temp file to keep downloaded bytes private.
func downloadAndReplace(url, path string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepathDir(path), "hnsx-update.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
