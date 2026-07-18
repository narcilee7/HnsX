package agentruntime

import "log/slog"

// NewDefaultRegistry constructs a registry pre-loaded with every
// backend ported so far. app.New uses this to wire the daemon_runtime.
func NewDefaultRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	r := NewRegistry(logger)

	// Stream-json style — fully ported.
	r.Register(NewClaudeBackend(NewClaudeRunner("", logger)))
	r.Register(NewCursorBackend())
	r.Register(NewCopilotBackend())
	r.Register(NewCodeBuddyBackend())
	r.Register(NewQoderBackend())
	r.Register(NewTraeCLIBackend())
	r.Register(NewAntigravityBackend())
	r.Register(NewOpenCodeBackend())
	r.Register(NewPiBackend())
	r.Register(NewCodexBackend())

	// ACP / JSON-RPC style — R3.5g ports hermes/kimi/kiro.
	r.Register(NewHermesBackend())
	r.Register(NewKimiBackend())
	r.Register(NewKiroBackend())

	// openclaw still a stub: in-process agent runtime + gateway routing
	// warrants its own effort; R2.x.
	r.Register(NewOpenClawBackend())

	return r
}