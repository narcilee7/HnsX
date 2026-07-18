package main

import (
	"github.com/hnsx-io/hnsx/server/internal/app"
)

// minimalConfig returns a config usable by subcommands that only need
// app.New to wire the agent runtime registry (not the HTTP server or DB).
// Keeps config loading lightweight for `hnsxd backends list` style commands.
func minimalConfig() *app.Config {
	cfg, err := app.LoadConfig("")
	if err != nil {
		// LoadConfig with an empty path should never fail; if it does,
		// surface a minimal config so the caller can still proceed.
		return &app.Config{HTTPAddr: ":8080", LogLevel: "info"}
	}
	return cfg
}