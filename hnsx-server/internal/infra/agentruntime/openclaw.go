package agentruntime

// OpenClawBackend is a stub. The full OpenClaw integration includes a
// complex runtime (in-process agent loop, gateway routing, MCP overlays)
// that warrants its own R2.x effort.
//
// Reference: git show 4aedded:multica_fork/pkg/agent/openclaw.go (702 lines)
func NewOpenClawBackend() *StubBackend {
	return NewStubBackend("openclaw", "openclaw",
		"in-process agent runtime not yet ported (R2.x)")
}