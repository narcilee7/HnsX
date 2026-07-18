package agentruntime

// KimiBackend implements agentruntime.Backend for kimi-cli via ACP.
//
// Reference: git show 4aedded:multica_fork/pkg/agent/kimi.go
func NewKimiBackend() *ACPBackend {
	return NewACPBackend(ACPConfig{
		Name:      "kimi",
		Executable: "kimi",
		ExtraArgs: nil,
	})
}
