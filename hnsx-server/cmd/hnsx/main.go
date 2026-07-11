// hnsx is the operator-facing CLI for HnsX. It exposes local commands that
// do not require a running control plane (validate / run / format) and remote
// commands that talk to the server API (domain / session / eval / trace).
//
// As of v0.3 ("Lifesaver") the CLI is built on cobra + pflag and adds
// lifecycle (up / down / status / doctor / logs / reset) and discovery
// (try / examples / completion) subcommands.
//
// Subcommand tree (see docs/cli-roadmap.md for the full vocabulary):
//
//   Lifecycle   up | down | restart | status | doctor | logs | reset
//   Discovery   try | examples | completion
//   Local       validate | run
//   Remote      remote <domains|sessions|evals> ...    (deprecated)
//   Meta        version | help
package main

import (
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}