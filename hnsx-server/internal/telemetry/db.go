package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/trace/repository"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// DBSink persists each observation into the `observations` table created by
// go/migrations/000002_observations.up.sql. It is intended for the local
// "no OTLP" mode where operators want basic query/replay.
type DBSink struct {
	db     *gorm.DB
	schema string // override for testing; default public
}

// NewDBSink creates a DBSink against the given GORM DB.
func NewDBSink(db *gorm.DB) *DBSink {
	return &DBSink{db: db}
}

// WithSchema configures the target schema (default "public").
func (s *DBSink) WithSchema(schema string) *DBSink {
	s.schema = schema
	return s
}

// Name returns "db".
func (s *DBSink) Name() string { return "db" }

// Record inserts one row per observation. Errors are returned (the runner
// does not currently surface sink errors but a future telemetry-aware
// control loop can use them).
func (s *DBSink) Record(ctx context.Context, obs runtime.Observation) error {
	if s == nil || s.db == nil {
		return nil
	}
	schema := s.schema
	if schema == "" {
		schema = "public"
	}

	payload := obs.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("db sink: marshal payload: %w", err)
	}

	record := repository.ObservationRecord{
		TraceID:   obs.TraceID,
		SessionID: obs.SessionID,
		DomainID:  obs.DomainID,
		StepID:    obs.StepID,
		AgentID:   obs.AgentID,
		Kind:      obs.Kind,
		Payload:   body,
	}
	if obs.Cost != nil {
		record.CostUSD = obs.Cost.CostUSD
		record.PromptTokens = obs.Cost.PromptTokens
		record.CompletionTokens = obs.Cost.CompletionTokens
		record.LatencyMs = obs.Cost.LatencyMs
	}
	if !obs.Timestamp.IsZero() {
		record.CreatedAt = obs.Timestamp
	} else {
		record.CreatedAt = time.Now().UTC()
	}

	table := fmt.Sprintf("%s.observations", schema)
	if err := s.db.WithContext(ctx).Table(table).Create(&record).Error; err != nil {
		return fmt.Errorf("db sink: insert: %w", err)
	}
	return nil
}

// Flush is a no-op for now.
func (s *DBSink) Flush(_ context.Context) error { return nil }

// Close is a no-op; the DB lifecycle is owned by the host server.
func (s *DBSink) Close(_ context.Context) error { return nil }
