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

	// ACP / JSON-RPC style — stub for R2 (returns clear "not ported" error).
	r.Register(NewHermesBackend())
	r.Register(NewKimiBackend())
	r.Register(NewKiroBackend())
	r.Register(NewOpenClawBackend())

	return r
}