package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

// newRemoteCmd in v1.0 is a hidden alias group that prints a clear
// migration message and re-exports the canonical resource commands. The
// tree will be removed in v1.1 (one-cycle grace window per the roadmap's
// deprecation policy).
func newRemoteCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "remote",
		Short:  "Deprecated alias tree; use resource commands (hnsx domain, session, eval, trace)",
		Hidden: true,
		Long: `DEPRECATION (v1.0): 'hnsx remote <x>' has been folded into the
resource-oriented command tree. As of v1.0 the supported equivalents are:

  hnsx remote domains list          →  hnsx domain list
  hnsx remote domains get <id>      →  hnsx domain show <id>
  hnsx remote domains register      →  hnsx domain register
  hnsx remote sessions list         →  hnsx session list
  hnsx remote sessions get <id>     →  hnsx session show <id>
  hnsx remote sessions trigger      →  hnsx session trigger
  hnsx remote sessions cancel       →  hnsx session cancel
  hnsx remote sessions rerun        →  hnsx session rerun
  hnsx remote sessions events <id>  →  hnsx session tail <id>
  hnsx remote evals list            →  hnsx eval set list
  hnsx remote evals run             →  hnsx eval run start

This 'remote' tree will be removed in v1.1.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(os.Stderr,
				"⚠ 'hnsx remote <x>' is deprecated and hidden; prefer the resource-oriented command (e.g. `hnsx domain list`). Removal: v1.1.")
		},
	}
	cmd.AddCommand(newRemoteDomains(cfg))
	cmd.AddCommand(newRemoteSessions(cfg))
	cmd.AddCommand(newRemoteEvals(cfg))
	return cmd
}

func newRemoteDomains(cfg *Config) *cobra.Command {
	c := client.New()
	cmd := &cobra.Command{Use: "domains", Short: "Domain operations"}
	list := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := c.ListDomains()
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(items)
		},
	}
	get := &cobra.Command{
		Use:  "get <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := c.GetDomain(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(d)
		},
	}
	var file string
	register := &cobra.Command{
		Use: "register --file <path>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			d, err := c.RegisterDomain(f, "application/x-yaml")
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(d)
		},
	}
	register.Flags().StringVar(&file, "file", "", "path to domain YAML")
	cmd.AddCommand(list, get, register)
	return cmd
}

func newRemoteSessions(cfg *Config) *cobra.Command {
	c := client.New()
	cmd := &cobra.Command{Use: "sessions", Short: "Session operations"}
	list := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := c.ListSessions()
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(items)
		},
	}
	get := &cobra.Command{
		Use:  "get <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := c.GetSession(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(s)
		},
	}
	var domainID, trigger string
	triggerCmd := &cobra.Command{
		Use: "trigger --domain <id> [--trigger <json>]",
		RunE: func(cmd *cobra.Command, args []string) error {
			if domainID == "" {
				return fmt.Errorf("--domain is required")
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(trigger), &payload); err != nil {
				return err
			}
			s, err := c.TriggerSession(domainID, payload)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(s)
		},
	}
	triggerCmd.Flags().StringVar(&domainID, "domain", "", "domain id")
	triggerCmd.Flags().StringVar(&trigger, "trigger", "{}", "JSON trigger payload")
	cancel := &cobra.Command{
		Use:  "cancel <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := c.CancelSession(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(s)
		},
	}
	rerun := &cobra.Command{
		Use:  "rerun <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := c.RerunSession(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(s)
		},
	}
	events := &cobra.Command{
		Use:  "events <id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancelCtx := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancelCtx()
			evs, errCh, err := c.SessionEvents(ctx, args[0])
			if err != nil {
				return err
			}
			for {
				select {
				case evt, ok := <-evs:
					if !ok {
						return nil
					}
					fmt.Printf("event: %s\ndata: %s\n\n", evt.Name, string(evt.Payload))
				case err := <-errCh:
					if err != nil {
						return err
					}
					return nil
				}
			}
		},
	}
	cmd.AddCommand(list, get, triggerCmd, cancel, rerun, events)
	return cmd
}

func newRemoteEvals(cfg *Config) *cobra.Command {
	c := client.New()
	cmd := &cobra.Command{Use: "evals", Short: "Eval operations"}
	list := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := c.ListEvalSets()
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(items)
		},
	}
	cmd.AddCommand(list)
	// Other eval subcommands (create/get/update/delete/run/runs ...) are
	// intentionally not re-implemented here; users are pointed at the new
	// `hnsx eval ...` tree in the deprecation notice.
	return cmd
}
