package agentruntime

// KiroBackend is a stub. Real implementation requires ACP backend base.
// Reference: git show 4aedded:multica_fork/pkg/agent/kiro.go
func NewKiroBackend() *StubBackend {
	return NewStubBackend("kiro", "kiro-cli",
		"ACP/JSON-RPC transport not yet ported (R2.x)")
}