package agentruntime

// HermesBackend is a stub. The real Hermes CLI speaks ACP (Agent Client
// Protocol) JSON-RPC over stdin/stdout — not stream-json — so porting
// requires a separate ACP backend base. Tracked for R2.x.
//
// Reference: git show 4aedded:multica_fork/pkg/agent/hermes.go
func NewHermesBackend() *StubBackend {
	return NewStubBackend("hermes", "hermes",
		"ACP/JSON-RPC transport not yet ported (R2.x)")
}