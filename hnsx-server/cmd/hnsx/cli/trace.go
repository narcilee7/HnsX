package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newTraceCmd provides the trace-list / trace-show / trace-export surface.
// The server exposes traces under /api/v1/traces; we hit it directly because
// the legacy client does not yet have a typed Trace view.
func newTraceCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Inspect traces and observations",
	}
	cmd.AddCommand(newTraceListCmd(cfg))
	cmd.AddCommand(newTraceShowCmd(cfg))
	cmd.AddCommand(newTraceExportCmd(cfg))
	return cmd
}

// traceListItem mirrors the JSON envelope of GET /api/v1/traces. The server
// returns "trace_id" and "session_id" rather than "id", so we use json tags
// that match exactly.
type traceListItem struct {
	ID         string  `json:"trace_id"`
	SessionID  string  `json:"session_id"`
	DomainID   string  `json:"domain_id"`
	StartedAt  string  `json:"started_at"`
	DurationMS int64   `json:"duration_ms"`
	Cost       float64 `json:"total_cost_usd"`
}

func newTraceListCmd(cfg *Config) *cobra.Command {
	var (
		session  string
		domain   string
		since    string
		costMin  string
		limit    int
		filters  []string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := fetchTraceList(cfg)
			if err != nil {
				return err
			}
			sinceT, err := parseSince(since)
			if err != nil {
				return err
			}
			fmap, err := parseFilters(filters)
			if err != nil {
				return err
			}
			out := make([]traceListItem, 0, len(items))
			for _, it := range items {
				if session != "" && it.SessionID != session {
					continue
				}
				if domain != "" && it.DomainID != domain {
					continue
				}
				if costMin != "" && it.Cost > 0 {
					if fmt.Sprintf("%f", it.Cost) < costMin {
						continue
					}
				}
				if !sinceT.IsZero() && it.StartedAt != "" {
					if t, perr := parseRFC3339(it.StartedAt); perr == nil && t.Before(sinceT) {
						continue
					}
				}
				if !filterMatches(map[string]string{
					"id":         it.ID,
					"session_id": it.SessionID,
					"domain_id":  it.DomainID,
				}, fmap) {
					continue
				}
				out = append(out, it)
				if limit > 0 && len(out) >= limit {
					break
				}
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(out)
				return nil
			}
			if cfg.Output == "quiet" {
				for _, t := range out {
					fmt.Println(t.ID)
				}
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, t := range out {
				rows = append(rows, []string{
					t.ID,
					t.SessionID,
					t.DomainID,
					shortTime(t.StartedAt),
					fmt.Sprintf("%dms", t.DurationMS),
					fmt.Sprintf("$%.4f", t.Cost),
				})
			}
			o.Table([]string{"ID", "SESSION", "DOMAIN", "STARTED", "DURATION", "COST"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "filter by session id")
	cmd.Flags().StringVar(&domain, "domain", "", "filter by domain id")
	cmd.Flags().StringVar(&since, "since", "", "filter by start time")
	cmd.Flags().StringVar(&costMin, "cost-min", "", "filter by minimum cost")
	cmd.Flags().IntVar(&limit, "limit", 50, "max number of items (0 = unlimited)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "filter as key=value")
	return cmd
}

func newTraceShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <trace-id>",
		Short: "Show a trace tree of observations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := getJSON(cfg, "/api/v1/traces/"+args[0])
			if err != nil {
				return err
			}
			if cfg.Output == "json" {
				fmt.Println(string(body))
				return nil
			}
			// Pretty-print indented JSON; observation tree details follow.
			var pretty map[string]any
			if err := json.Unmarshal(body, &pretty); err == nil {
				out, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(out))
			} else {
				fmt.Println(string(body))
			}
			return nil
		},
	}
	return cmd
}

func newTraceExportCmd(cfg *Config) *cobra.Command {
	var (
		format string
		out    string
	)
	cmd := &cobra.Command{
		Use:   "export <trace-id> [--format json|yaml|otlp]",
		Short: "Export a trace for offline analysis",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := getJSON(cfg, "/api/v1/traces/"+args[0])
			if err != nil {
				return err
			}
			w := os.Stdout
			if out != "" {
				f, err := os.Create(out)
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			switch strings.ToLower(format) {
			case "json":
				_, _ = w.Write(body)
			case "yaml":
				// JSON → YAML: best-effort via a quick remarshal.
				pretty, _ := json.MarshalIndent(json.RawMessage(body), "", "  ")
				fmt.Fprintln(w, "# yaml export not yet implemented; emitted JSON instead:")
				_, _ = w.Write(pretty)
			case "otlp":
				fmt.Fprintln(w, "# otlp export not yet implemented; emitted JSON instead:")
				_, _ = w.Write(body)
			default:
				return fmt.Errorf("invalid --format %q (want json|yaml|otlp)", format)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "export format: json|yaml|otlp")
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file (default stdout)")
	return cmd
}

// fetchTraceList GETs /api/v1/traces and returns the `items` array.
// The endpoint returns {"items":[...], "limit":N, "offset":N, "total":N}.
func fetchTraceList(cfg *Config) ([]traceListItem, error) {
	body, err := getJSON(cfg, "/api/v1/traces")
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []traceListItem `json:"items"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse traces response: %w", err)
	}
	if env.Items == nil {
		return []traceListItem{}, nil
	}
	return env.Items, nil
}

// getJSON is a tiny convenience around http.Get with sane timeout and
// HTTP-error handling.
func getJSON(cfg *Config, path string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, cfg.ServerURL+path, nil)
	if err != nil {
		return nil, err
	}
	withAuth(cfg, req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// withAuth attaches the configured bearer token to an outgoing request.
func withAuth(cfg *Config, req *http.Request) {
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
}

// doAuthorized sends an HTTP request with the configured auth token.
func doAuthorized(cfg *Config, req *http.Request) (*http.Response, error) {
	withAuth(cfg, req)
	return http.DefaultClient.Do(req)
}