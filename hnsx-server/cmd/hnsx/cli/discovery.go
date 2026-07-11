package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ExampleDomain describes one discoverable example domain shipped with HnsX.
type ExampleDomain struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Description string   `json:"description,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// TemplateIndex is the shape of templates/index.yaml.
type TemplateIndex struct {
	Version   string          `yaml:"version"`
	Templates []TemplateEntry `yaml:"templates"`
}

// TemplateEntry describes one template in the index.
type TemplateEntry struct {
	ID           string               `yaml:"id"`
	Name         string               `yaml:"name"`
	Description  string               `yaml:"description"`
	Tags         []string             `yaml:"tags"`
	Source       string               `yaml:"source"`
	Variables    []TemplateVariable   `yaml:"variables"`
	Requirements TemplateRequirements `yaml:"requirements"`
}

// TemplateVariable is a user-settable placeholder in a template.
type TemplateVariable struct {
	Name    string `yaml:"name"`
	Default string `yaml:"default"`
}

// TemplateRequirements describes runtime prerequisites.
type TemplateRequirements struct {
	Providers       []string `yaml:"providers"`
	MinWorkers      int      `yaml:"min_workers"`
	SandboxRuntimes []string `yaml:"sandbox_runtimes"`
}

// loadTemplateIndex reads <repoRoot>/templates/index.yaml if it exists.
func loadTemplateIndex(cfg *Config) (*TemplateIndex, error) {
	path := filepath.Join(cfg.RepoRoot, "templates", "index.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TemplateIndex{}, nil
		}
		return nil, fmt.Errorf("read template index: %w", err)
	}
	var idx TemplateIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse template index: %w", err)
	}
	return &idx, nil
}

// discoverExamples scans <repoRoot>/templates/index.yaml (preferred) and falls
// back to scanning <repoRoot>/example-domains/ for any template not in the
// index. Failures on individual files are reported via stderr but do not abort
// the listing.
func discoverExamples(cfg *Config) ([]ExampleDomain, error) {
	idx, err := loadTemplateIndex(cfg)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]ExampleDomain)
	for _, t := range idx.Templates {
		path := filepath.Join(cfg.RepoRoot, t.Source)
		if _, err := os.Stat(path); err != nil {
			// Best-effort: if the indexed source is missing, keep the entry but
			// note it may fail at try/init time.
			path = t.Source
		}
		mode := ""
		if b, err := os.ReadFile(path); err == nil {
			var doc struct {
				Harness struct {
					Session struct {
						Mode string `yaml:"mode"`
					} `yaml:"session"`
				} `yaml:"harness"`
			}
			if err := yaml.Unmarshal(b, &doc); err == nil {
				mode = doc.Harness.Session.Mode
			}
		}
		byName[t.ID] = ExampleDomain{
			Name:        t.ID,
			Path:        path,
			Description: strings.SplitN(t.Description, "\n", 2)[0],
			Mode:        mode,
			Tags:        t.Tags,
		}
	}

	// Fall back to scanning example-domains for anything not indexed.
	dir := filepath.Join(cfg.RepoRoot, "example-domains")
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name(), "domain.yaml")
			if _, err := os.Stat(path); err != nil {
				continue
			}
			name := e.Name()
			if _, ok := byName[name]; ok {
				continue
			}
			ex := ExampleDomain{Name: name, Path: path}
			if b, err := os.ReadFile(path); err == nil {
				var doc struct {
					ID          string `yaml:"id"`
					Description string `yaml:"description"`
					Harness     struct {
						Session struct {
							Mode string `yaml:"mode"`
						} `yaml:"session"`
					} `yaml:"harness"`
				}
				if err := yaml.Unmarshal(b, &doc); err == nil {
					if doc.ID != "" {
						ex.Name = doc.ID
					}
					ex.Description = strings.SplitN(doc.Description, "\n", 2)[0]
					ex.Mode = doc.Harness.Session.Mode
				}
			}
			byName[ex.Name] = ex
		}
	}

	out := make([]ExampleDomain, 0, len(byName))
	for _, ex := range byName {
		out = append(out, ex)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func matchesTag(ex ExampleDomain, tag string) bool {
	if tag == "" {
		return true
	}
	for _, t := range ex.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// hnsx examples
// ---------------------------------------------------------------------------

func newExamplesCmd(cfg *Config) *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "examples",
		Short: "List the example domains shipped with HnsX",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			items, err := discoverExamples(cfg)
			if err != nil {
				return err
			}
			var filtered []ExampleDomain
			for _, it := range items {
				if matchesTag(it, tag) {
					filtered = append(filtered, it)
				}
			}
			if cfg.Output == "json" {
				out.Print(filtered)
				return nil
			}
			if cfg.Output == "quiet" {
				for _, e := range filtered {
					fmt.Println(e.Name)
				}
				return nil
			}
			headers := []string{"NAME", "MODE", "TAGS", "DESCRIPTION"}
			rows := make([][]string, 0, len(filtered))
			for _, e := range filtered {
				desc := e.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				tags := strings.Join(e.Tags, ", ")
				rows = append(rows, []string{e.Name, e.Mode, tags, desc})
			}
			out.Table(headers, rows)
			out.Line("\n→ run one with: hnsx try <name>")
			return nil
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "filter examples by tag")
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx try <name>
// ---------------------------------------------------------------------------

func newTryCmd(cfg *Config) *cobra.Command {
	var trigger string
	var detach bool
	cmd := &cobra.Command{
		Use:   "try <name>",
		Short: "Try an example domain end-to-end (up + register + trigger + tail)",
		Long: `Convenience for first-time users: ensures the stack is running,
registers the chosen example domain, fires a session with --trigger
(default {"question":"hello"}), and tails the SSE event stream until the
session completes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			name := args[0]

			examples, err := discoverExamples(cfg)
			if err != nil {
				return err
			}
			var ex *ExampleDomain
			for i := range examples {
				if examples[i].Name == name {
					ex = &examples[i]
					break
				}
			}
			if ex == nil {
				return fmt.Errorf("unknown example %q (run `hnsx examples` to list)", name)
			}

			// 1. ensure server is healthy (auto-up if not).
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if err := httpHealth(ctx, cfg.ServerURL); err != nil {
				out.Line("→ server not healthy, running `hnsx up` first ...")
				upArgs := []string{"up", "-d", "postgres", "server", "worker"}
				if err := runCompose(cfg, upArgs...); err != nil {
					return fmt.Errorf("auto-up failed: %w", err)
				}
				if err := waitHealthy(cfg, 60*time.Second); err != nil {
					return err
				}
			}

			// 2. register the domain (PUT if it already exists, POST otherwise).
			out.Line("→ registering %s from %s", ex.Name, ex.Path)
			if err := registerOrUpdate(cfg, ex.Name, ex.Path); err != nil {
				return fmt.Errorf("register domain: %w", err)
			}

			// 3. trigger a session.
			out.Line("→ triggering session (trigger=%s)", trigger)
			triggerPayload := fmt.Sprintf(`{"domain_id":"%s","trigger":%s}`, ex.Name, trigger)
			resp, err := postJSON(cfg, "/api/v1/sessions", []byte(triggerPayload))
			if err != nil {
				return fmt.Errorf("trigger session: %w", err)
			}
			var session struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(resp, &session); err != nil {
				return fmt.Errorf("parse session response: %w", err)
			}
			out.Line("✓ session %s started", session.ID)

			if detach {
				return nil
			}

			// 4. tail SSE until complete (best-effort; the server closes the
			//    stream when the session terminates).
			out.Line("→ tailing events (Ctrl-C to stop)")
			return tailEvents(cfg, session.ID)
		},
	}
	cmd.Flags().StringVar(&trigger, "trigger", `{"question":"hello"}`, "JSON trigger payload")
	cmd.Flags().BoolVar(&detach, "detach", false, "do not tail events after triggering")
	return cmd
}

// waitHealthy polls /healthz until success or timeout.
func waitHealthy(cfg *Config, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := httpHealth(ctx, cfg.ServerURL)
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("server did not become healthy within %s", d)
}

// postFile POSTs a YAML file to a server endpoint with content-type
// application/x-yaml.
func postFile(cfg *Config, path string, body *os.File) error {
	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// putFile PUTs a YAML file to a server endpoint (used for updates).
func putFile(cfg *Config, path string, body *os.File) error {
	req, err := http.NewRequest(http.MethodPut, cfg.ServerURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// httpNewDelete builds a DELETE request (used by resource commands that need
// an HTTP verb the legacy client doesn't expose).
func httpNewDelete(url string) (*http.Request, error) {
	return http.NewRequest(http.MethodDelete, url, nil)
}

// httpDo executes an http.Request and returns the response.
func httpDo(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

// registerOrUpdate registers the domain. If it already exists (server returns
// 400 with VALIDATION_FAILED / "already exists"), we treat that as success to
// keep `hnsx try` idempotent. This makes re-running the same example work
// without requiring a fresh DB.
func registerOrUpdate(cfg *Config, name, path string) error {
	body, err := os.Open(path)
	if err != nil {
		return err
	}
	defer body.Close()

	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+"/api/v1/domains", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	// 400 with "already exists" means the domain is registered — that's fine
	// for an idempotent try. Other 4xx/5xx are real errors.
	if resp.StatusCode == http.StatusBadRequest {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		if strings.Contains(string(buf[:n]), "already exists") {
			return nil
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(buf[:n])))
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

// postJSON sends a JSON POST and returns the response body.
func postJSON(cfg *Config, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return readAll(resp.Body)
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var b []byte
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b = append(b, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return b, nil
			}
			return b, err
		}
	}
}

// tailEvents consumes the SSE stream for a session and prints events.
func tailEvents(cfg *Config, sessionID string) error {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/v1/sessions/%s/events", cfg.ServerURL, sessionID), nil)
	if err != nil {
		return err
	}
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("events: HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	return scanner.Err()
}

// ---------------------------------------------------------------------------
// hnsx completion
// ---------------------------------------------------------------------------

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Output a shell completion script for the given shell to stdout.

Examples:
  hnsx completion bash | sudo tee /etc/bash_completion.d/hnsx > /dev/null
  hnsx completion zsh > "${fpath[1]}/_hnsx"
  hnsx completion fish > ~/.config/fish/completions/hnsx.fish
  hnsx completion powershell | Out-File -Encoding utf8 hnsx.ps1
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}
