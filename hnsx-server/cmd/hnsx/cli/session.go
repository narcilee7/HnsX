package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

func newSessionCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage Session resources",
	}
	cmd.AddCommand(newSessionListCmd(cfg))
	cmd.AddCommand(newSessionShowCmd(cfg))
	cmd.AddCommand(newSessionTriggerCmd(cfg))
	cmd.AddCommand(newSessionCancelCmd(cfg))
	cmd.AddCommand(newSessionRerunCmd(cfg))
	cmd.AddCommand(newSessionTailCmd(cfg))
	cmd.AddCommand(newSessionApproveCmd(cfg))
	cmd.AddCommand(newSessionRejectCmd(cfg))
	return cmd
}

// stateColor maps a session state to a one-character status glyph so the
// human-mode table can show progress at a glance.
func stateColor(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return "●"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "paused", "pending_approval":
		return "⏸"
	case "cancelled", "canceled":
		return "⊘"
	default:
		return "?"
	}
}

func newSessionListCmd(cfg *Config) *cobra.Command {
	var (
		domain  string
		state   string
		since   string
		limit   int
		filters []string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			items, err := c.ListSessions()
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
			out := make([]client.SessionListItem, 0, len(items))
			for _, it := range items {
				if domain != "" && it.DomainID != domain {
					continue
				}
				if state != "" && !strings.EqualFold(it.State, state) {
					continue
				}
				if !sinceT.IsZero() && it.StartedAt.Before(sinceT) {
					continue
				}
				if !filterMatches(map[string]string{
					"id":        it.ID,
					"domain_id": it.DomainID,
					"state":     it.State,
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
				for _, s := range out {
					o.Line("%s", s.ID)
				}
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, s := range out {
				rows = append(rows, []string{
					stateColor(s.State),
					s.ID,
					s.DomainID,
					nonEmpty(s.State, "-"),
					shortTime(s.StartedAt),
					shortTimePtr(s.CompletedAt),
				})
			}
			o.Table([]string{"", "ID", "DOMAIN", "STATE", "STARTED", "COMPLETED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "filter by domain id")
	cmd.Flags().StringVar(&state, "state", "", "filter by state (running|completed|failed|paused|cancelled)")
	cmd.Flags().StringVar(&since, "since", "", "filter by start time (e.g. 5m, 1h, 2d)")
	cmd.Flags().IntVar(&limit, "limit", 50, "max number of items (0 = unlimited)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "filter as key=value")
	return cmd
}

func newSessionShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a single session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			s, err := c.GetSession(args[0])
			if err != nil {
				return err
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(s)
				return nil
			}
			if cfg.Output == "quiet" {
				o.Line("%s", s.ID)
				return nil
			}
			o.Card("Session", [][2]string{
				{"id", s.ID},
				{"domain", s.DomainID},
				{"version", nonEmpty(s.DomainVersion, "-")},
				{"state", s.State},
				{"orchestration", nonEmpty(s.Orchestration, "-")},
				{"started", formatTime(s.StartedAt, "-")},
				{"completed", formatTimePtr(s.CompletedAt, "-")},
			})
			if len(s.Trigger) > 0 {
				o.Section("Trigger")
				o.Print(s.Trigger)
			}
			if len(s.Result) > 0 {
				o.Section("Result")
				o.Print(s.Result)
			}
			return nil
		},
	}
	return cmd
}

func newSessionTriggerCmd(cfg *Config) *cobra.Command {
	var (
		domainID string
		trigger  string
		async    bool
	)
	cmd := &cobra.Command{
		Use:   "trigger --domain <id> [--trigger <json|@file>]",
		Short: "Trigger a new session",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			if domainID == "" {
				return errors.New("--domain is required")
			}
			payload, err := loadTrigger(trigger)
			if err != nil {
				return err
			}
			s, err := c.TriggerSession(domainID, payload)
			if err != nil {
				return err
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "quiet" {
				o.Line("%s", s.ID)
				return nil
			}
			o.Print(s)
			if async {
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&domainID, "domain", "", "domain id")
	cmd.Flags().StringVar(&trigger, "trigger", "{}", "JSON trigger payload, or @file.json")
	cmd.Flags().BoolVar(&async, "async", false, "do not tail events after triggering")
	return cmd
}

// loadTrigger parses a --trigger flag value: either inline JSON or @file.json.
func loadTrigger(s string) (map[string]any, error) {
	if strings.HasPrefix(s, "@") {
		b, err := os.ReadFile(s[1:])
		if err != nil {
			return nil, fmt.Errorf("read trigger file: %w", err)
		}
		s = string(b)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("parse trigger: %w", err)
	}
	return out, nil
}

func newSessionCancelCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a running session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			s, err := c.CancelSession(args[0])
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(s)
			return nil
		},
	}
	return cmd
}

func newSessionRerunCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rerun <id>",
		Short: "Rerun a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			s, err := c.RerunSession(args[0])
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(s)
			return nil
		},
	}
	return cmd
}

// newSessionTailCmd consumes the SSE stream for a session and prints each
// event on its own line, color-coded by event kind. Ctrl-C exits.
func newSessionTailCmd(cfg *Config) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "tail <id>",
		Short: "Tail the live SSE event stream for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			ctx := cmd.Context()
			if !follow {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
			}
			evs, errCh, err := c.SessionEvents(ctx, args[0])
			if err != nil {
				return err
			}
			out := NewOutput(cfg.Output)
			for {
				select {
				case ev, ok := <-evs:
					if !ok {
						return nil
					}
					printEvent(out, ev.Name, ev.Payload)
				case err := <-errCh:
					if err != nil && !errors.Is(err, io.EOF) {
						return err
					}
					return nil
				case <-ctx.Done():
					return nil
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep tailing until Ctrl-C (default timeout 5m)")
	return cmd
}

// printEvent renders one SSE event to stdout with a colored prefix.
func printEvent(out *Output, name string, payload []byte) {
	prefix := "•"
	switch name {
	case "observation.created":
		prefix = "+"
	case "observation.text":
		prefix = ">"
	case "observation.tool_call":
		prefix = "⚒"
	case "observation.error":
		prefix = "✗"
	case "session.completed":
		prefix = "✓"
	case "session.failed":
		prefix = "✗"
	}
	out.Line("%s %s %s", prefix, name, truncate(string(payload), 200))
}

func newSessionApproveCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a paused session awaiting human-in-the-loop",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return postApproval(cfg, args[0], "approve")
		},
	}
	return cmd
}

func newSessionRejectCmd(cfg *Config) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <id> --reason <text>",
		Short: "Reject a paused session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return postApproval(cfg, args[0], "reject")
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "rejection reason")
	return cmd
}

// postApproval hits the approval endpoint (path kept flexible; matches the
// server's v1 approval route).
func postApproval(cfg *Config, sessionID, action string) error {
	url := cfg.ServerURL + "/api/v1/approvals/" + sessionID + "/" + action
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := doAuthorized(cfg, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	NewOutput(cfg.Output).Line("✓ %s %s", action, sessionID)
	return nil
}

// parseRFC3339 accepts both RFC3339Nano and RFC3339.
func parseRFC3339(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
