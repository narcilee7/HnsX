// Package version holds HnsX version info. The three vars are populated at
// link time via -ldflags by the Makefile build target.
package version

// Version is the semantic version of the build.
var Version = "0.2.0"

// Commit is the git short hash the binary was built from.
var Commit = "unknown"

// Built is the RFC3339 timestamp the binary was built at.
var Built = "unknown"

// String returns a one-line description of the build.
func String() string {
	return "hnsx " + Version + " (commit " + Commit + ", built " + Built + ")"
}
