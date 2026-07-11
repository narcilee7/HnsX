package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newConfigCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage hnsx CLI configuration",
	}
	cmd.AddCommand(newConfigShowCmd(cfg))
	cmd.AddCommand(newConfigGetCmd(cfg))
	cmd.AddCommand(newConfigSetCmd(cfg))
	return cmd
}

func newConfigShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show effective CLI configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(cfg.ToMap())
				return nil
			}
			if cfg.Output == "quiet" {
				fmt.Println(cfg.ConfigFile)
				return nil
			}
			o.Card("Effective Configuration", configPairs(cfg))
			return nil
		},
	}
	return cmd
}

func newConfigGetCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a single config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := cfg.Get(args[0])
			if err != nil {
				return err
			}
			if cfg.Output == "quiet" {
				fmt.Println(v)
				return nil
			}
			fmt.Println(v)
			return nil
		},
	}
	return cmd
}

func newConfigSetCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value and persist to ~/.config/hnsx/config.yaml",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.SaveToFile(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			o := NewOutput(cfg.Output)
			o.Line("✓ %s = %s", args[0], args[1])
			return nil
		},
	}
	return cmd
}

func configPairs(cfg *Config) [][2]string {
	m := cfg.ToMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([][2]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, [2]string{k, m[k]})
	}
	return pairs
}
