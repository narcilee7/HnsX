package agentruntime

import "github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"

// HermesBackend implements agentruntime.Backend for hermes-cli using
// the ACP (JSON-RPC 2.0) wire protocol.
//
// Reference: git show 4aedded:multica_fork/pkg/agent/hermes.go
func NewHermesBackend() *ACPBackend {
	return NewACPBackend(ACPConfig{
		Name:       "hermes",
		Executable: "hermes",
		ExtraArgs:  nil,
	})
}

var _ agentruntime.Backend = (*ACPBackend)(nil)

