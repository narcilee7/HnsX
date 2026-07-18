package agentruntime

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
)

// stderrTail is an io.Writer that captures the last N bytes written to it
// and forwards each line to the slog logger. Used to attribute CLI errors
// to a specific subprocess run while keeping RAM bounded.
//
// The contract is "best effort": if more than N bytes arrive, the oldest
// are dropped silently. Reads are not supported; this is a sink only.
type stderrTail struct {
	logger *slog.Logger
	cap    int
	mu     sync.Mutex
	buf    bytes.Buffer
}

// newStderrTail returns a writer that tees each line to logger.Error and
// keeps the trailing cap bytes available via Tail().
func newStderrTail(logger *slog.Logger, cap int) *stderrTail {
	if cap <= 0 {
		cap = 4 * 1024
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &stderrTail{logger: logger, cap: cap}
}

// Write splits incoming bytes into lines, logs each, and retains the tail.
func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, b := range p {
		s.buf.WriteByte(b)
		if b == '\n' {
			line := append([]byte(nil), s.buf.Bytes()...)
			s.logger.Error("claude: stderr", "line", string(bytes.TrimRight(line, "\n")))
			s.buf.Reset()
		}
	}
	if s.buf.Len() > s.cap {
		// Drop oldest bytes, keep last cap bytes.
		overflow := s.buf.Len() - s.cap
		s.buf.Next(overflow)
	}
	return len(p), nil
}

// Tail returns the trailing N bytes (or all if fewer were written).
// Safe to call concurrently.
func (s *stderrTail) Tail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// _ keeps io.Writer referenced for the linter; the assertion below is a
// compile-time check that stderrTail satisfies io.Writer.
var _ io.Writer = (*stderrTail)(nil)