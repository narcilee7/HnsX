package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print hnsx version info",
		Run: func(cmd *cobra.Command, args []string) {
			// Write through cobra's stdout stream so callers (tests,
			// $(...) substitution) can capture version info.
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
		},
	}
}
