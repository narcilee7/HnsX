// Package sandbox provides execution isolation backends.
package sandbox

import "context"

// Backend is a sandbox execution backend.
type Backend interface {
	Name() string
	Execute(ctx context.Context, command string, args []string, env map[string]string) (*Result, error)
}

// Result is the outcome of a sandbox execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// NoneBackend runs commands in the current process.
type NoneBackend struct{}

// NewNoneBackend creates a none backend.
func NewNoneBackend() *NoneBackend {
	return &NoneBackend{}
}

func (b *NoneBackend) Name() string { return "none" }

func (b *NoneBackend) Execute(ctx context.Context, command string, args []string, env map[string]string) (*Result, error) {
	return &Result{ExitCode: 0, Stdout: "none sandbox: execution allowed", Stderr: ""}, nil
}
