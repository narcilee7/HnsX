package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/go/internal/version"
	"github.com/hnsx-io/hnsx/go/pkg/controlplane"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		os.Exit(cmdServer(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version.String())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`hnsx-server — HnsX Control Plane

Usage:
  hnsx-server server --addr <addr>
  hnsx-server version

Examples:
  hnsx-server server --addr 127.0.0.1:50051
`)
}

func cmdServer(args []string) int {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:50051", "control plane listen address")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}

	server := controlplane.NewServer(*addr)
	fmt.Printf("hnsx control plane listening on %s\n", server.Addr())

	// TODO: implement gRPC/HTTP server startup.
	select {}
}
