package agentruntime

// KiroBackend implements agentruntime.Backend for kiro-cli via ACP.
//
// Reference: git show 4aedded:multica_fork/pkg/agent/kiro.go
//
// Kiro CLI 2.1+ uses --trust-all-tools to bypass its per-tool gate.
// The -a shorthand maps to --trust-all-tools (not --agent).
func NewKiroBackend() *ACPBackend {
	return NewACPBackend(ACPConfig{
		Name:      "kiro",
		Executable: "kiro-cli",
		ExtraArgs: []string{"--trust-all-tools"},
	})
}
