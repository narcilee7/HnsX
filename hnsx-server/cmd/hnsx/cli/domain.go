package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

func newDomainCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage Domain resources",
	}
	cmd.AddCommand(newDomainListCmd(cfg))
	cmd.AddCommand(newDomainShowCmd(cfg))
	cmd.AddCommand(newDomainRegisterCmd(cfg))
	cmd.AddCommand(newDomainValidateCmd(cfg))
	cmd.AddCommand(newDomainDeleteCmd(cfg))
	cmd.AddCommand(newDomainExportCmd(cfg))
	cmd.AddCommand(newDomainGroupFormatCmd(cfg))
	return cmd
}

// newDomainGroupFormatCmd exposes the formatter under the `domain` resource
// group as `hnsx domain format`. It reuses the implementation from
// `hnsx power format` so both paths stay identical.
func newDomainGroupFormatCmd(cfg *Config) *cobra.Command {
	return newDomainFormatCmd(cfg)
}

func newDomainListCmd(cfg *Config) *cobra.Command {
	var (
		limit   int
		filters []string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered domains",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			items, err := c.ListDomains()
			if err != nil {
				return err
			}
			fmap, err := parseFilters(filters)
			if err != nil {
				return err
			}
			// Apply filters + limit.
			out := make([]client.DomainListItem, 0, len(items))
			for _, it := range items {
				if !filterMatches(map[string]string{
					"id":     it.ID,
					"status": it.Status,
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
				for _, d := range out {
					fmt.Println(d.ID)
				}
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, d := range out {
				rows = append(rows, []string{
					d.ID,
					nonEmpty(d.Version, "-"),
					nonEmpty(d.Status, "-"),
					truncate(nonEmpty(d.Description, "-"), 50),
					shortTime(d.UpdatedAt),
				})
			}
			o.Table([]string{"ID", "VERSION", "STATUS", "DESCRIPTION", "UPDATED"}, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max number of items (0 = unlimited)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "filter as key=value (id=, status=)")
	return cmd
}

func newDomainShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>[@<version>]",
		Short: "Show a single domain's detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			id := args[0]
			d, err := c.GetDomain(id)
			if err != nil {
				return err
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(d)
				return nil
			}
			if cfg.Output == "quiet" {
				fmt.Println(d.ID)
				return nil
			}
			o.Card("Domain", [][2]string{
				{"id", d.ID},
				{"version", nonEmpty(d.Version, "-")},
				{"status", nonEmpty(d.Status, "-")},
				{"description", nonEmpty(d.Description, "-")},
				{"created", nonEmpty(d.CreatedAt, "-")},
				{"updated", nonEmpty(d.UpdatedAt, "-")},
			})
			if len(d.Harness) > 0 {
				o.Section("Harness")
				o.Print(d.Harness)
			}
			return nil
		},
	}
	return cmd
}

func newDomainRegisterCmd(cfg *Config) *cobra.Command {
	var files []string
	cmd := &cobra.Command{
		Use:   "register <path>...",
		Short: "Register one or more domain YAMLs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			files = args
			o := NewOutput(cfg.Output)
			for _, path := range files {
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("open %s: %w", path, err)
				}
				d, err := c.RegisterDomain(f, "application/x-yaml")
				f.Close()
				if err != nil {
					return fmt.Errorf("register %s: %w", path, err)
				}
				o.Line("✓ %s v%s", d.ID, d.Version)
			}
			return nil
		},
	}
	return cmd
}

func newDomainValidateCmd(cfg *Config) *cobra.Command {
	// Reuse the validate command from validate.go; just register it under the
	// domain group as an alias.
	return newValidateCmd(cfg)
}

func newDomainDeleteCmd(cfg *Config) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := NewOutput(cfg.Output)
			if !yes {
				o.Line("⚠ this will delete %s. Re-run with --yes to confirm.", args[0])
				return nil
			}
			// The legacy client lacks DeleteDomain(); call REST directly.
			req, err := httpNewDelete(cfg.ServerURL + "/api/v1/domains/" + args[0])
			if err != nil {
				return err
			}
			resp, err := httpDo(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			}
			o.Line("✓ deleted %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newDomainExportCmd(cfg *Config) *cobra.Command {
	var outFile string
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a domain's resolved spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			d, err := c.GetDomain(args[0])
			if err != nil {
				return err
			}
			if outFile == "" {
				fmt.Fprintf(os.Stdout, "%+v\n", d)
				return nil
			}
			f, err := os.Create(outFile)
			if err != nil {
				return err
			}
			defer f.Close()
			fmt.Fprintf(f, "%+v\n", d)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outFile, "out", "o", "", "output file (default stdout)")
	return cmd
}