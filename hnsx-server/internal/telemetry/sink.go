// Package telemetry centralizes HnsX telemetry sink implementations.
//
// The runtime emits domain.Observation values; the implementations in this
// package convert those into either OTLP spans/metrics, structured stdout,
// or DB rows. Sinks are designed to be plug-and-play: the runtime passes
// observations to every registered sink via runtime.Sink.
//
// Phase 1 ships:
//
//   - StdoutSink: prints observations as one JSON line per event.
//   - OtlpGRPCSink: ships observations as OTLP traces to a Collector.
//   - DBSink: persists observations into the `observations` table via GORM.
//   - FanOutSink: composes multiple sinks.
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// ----------------------------------------------------------------------------
// StdoutSink
// ----------------------------------------------------------------------------

// StdoutSink writes each observation as a single-line JSON record to the
// configured file (defaults to os.Stdout). Useful for local dev and CI.
type StdoutSink struct {
	mu  sync.Mutex
	out *os.File
	enc *json.Encoder
}

// NewStdoutSink constructs a sink that writes to os.Stdout.
func NewStdoutSink() *StdoutSink { return newStdoutSink(os.Stdout) }

func newStdoutSink(out *os.File) *StdoutSink {
	return &StdoutSink{out: out, enc: json.NewEncoder(out)}
}

// Name returns "stdout".
func (s *StdoutSink) Name() string { return "stdout" }

// Record writes one JSON line per observation.
func (s *StdoutSink) Record(_ context.Context, obs domain.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if obs.Timestamp.IsZero() {
		obs.Timestamp = time.Now().UTC()
	}
	return s.enc.Encode(obs)
}

// Flush is a no-op (writes are immediate).
func (s *StdoutSink) Flush(_ context.Context) error { return nil }

// Close is a no-op.
func (s *StdoutSink) Close(_ context.Context) error { return nil }

// ----------------------------------------------------------------------------
// FanOutSink
// ----------------------------------------------------------------------------

// FanOutSink dispatches every observation to one or more child sinks.
type FanOutSink struct {
	sinks []domain.Sink
}

// NewFanOutSink composes multiple sinks.
func NewFanOutSink(sinks ...domain.Sink) *FanOutSink { return &FanOutSink{sinks: sinks} }

// Name returns "fanout".
func (f *FanOutSink) Name() string { return "fanout" }

// Record forwards the observation to every child sink in order.
func (f *FanOutSink) Record(ctx context.Context, obs domain.Observation) error {
	for _, s := range f.sinks {
		if err := s.Record(ctx, obs); err != nil {
			return fmt.Errorf("sink %s: %w", s.Name(), err)
		}
	}
	return nil
}

// Flush forwards the Flush call to all children.
func (f *FanOutSink) Flush(ctx context.Context) error {
	for _, s := range f.sinks {
		if err := s.Flush(ctx); err != nil {
			return fmt.Errorf("sink %s flush: %w", s.Name(), err)
		}
	}
	return nil
}

// Close forwards the Close call to all children.
func (f *FanOutSink) Close(ctx context.Context) error {
	for _, s := range f.sinks {
		if err := s.Close(ctx); err != nil {
			return fmt.Errorf("sink %s close: %w", s.Name(), err)
		}
	}
	return nil
}
