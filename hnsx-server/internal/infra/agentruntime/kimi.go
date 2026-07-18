package agentruntime

// KimiBackend is a stub. Real implementation requires ACP backend base.
// Reference: git show 4aedded:multica_fork/pkg/agent/kimi.go
func NewKimiBackend() *StubBackend {
	return NewStubBackend("kimi", "kimi",
		"ACP/JSON-RPC transport not yet ported (R2.x)")
}