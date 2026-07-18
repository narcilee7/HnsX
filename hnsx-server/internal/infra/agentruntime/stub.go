package agentruntime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// StubBackend is a placeholder for backends that have been registered
// in the agentruntime port but whose concrete subprocess integration
// has not yet been ported from multica_fork. Calling Execute on a stub
// returns a clear "not yet implemented" error so operators see the gap
// rather than a silent nil.
//
// To replace a stub: copy the matching file from multica_fork/pkg/agent/
// (recoverable via git show 4aedded:multica_fork/pkg/agent/<name>.go),
// translate to the HnsX conventions (slog over zap, GORM over hand-rolled
// SQL, agentruntime.Session over *Session), and wire it into app.New.
type StubBackend struct {
	name      string
	execPath  string
	logger    *slog.Logger
	reasonMsg string
}

// NewStubBackend constructs a placeholder backend with a custom reason
// message surfaced when Execute is called.
func NewStubBackend(name, execPath, reason string) *StubBackend {
	return &StubBackend{name: name, execPath: execPath, reasonMsg: reason, logger: slog.Default()}
}

// Name implements agentruntime.Backend.
func (b *StubBackend) Name() string { return b.name }

// Execute implements agentruntime.Backend. Returns the stub reason.
func (b *StubBackend) Execute(ctx context.Context, prompt string, opts agentruntime.ExecOptions) (agentruntime.Session, error) {
	b.logger.Warn("agent: stub Execute called",
		"backend", b.name,
		"exec", b.execPath,
		"reason", b.reasonMsg,
	)
	return nil, fmt.Errorf("%s backend: %s", b.name, b.reasonMsg)
}

var _ agentruntime.Backend = (*StubBackend)(nil)