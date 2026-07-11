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
			// Write directly to os.Stdout: cobra's cmd.OutOrStdout is
			// captured by callers (e.g. tests, $(...) substitution),
			// and we want version info to always reach the terminal.
			fmt.Println(version.String())
		},
	}
}