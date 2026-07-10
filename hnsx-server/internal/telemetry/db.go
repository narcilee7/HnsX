package telemetry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// DBSink persists each observation into the `observations` table created by
// go/migrations/000002_observations.up.sql. It is intended for the local
// "no OTLP" mode where operators want basic query/replay.
type DBSink struct {
	pool   *pgxpool.Pool
	schema string // override for testing; default public
}

// NewDBSink creates a DBSink against the given pgxpool.
func NewDBSink(pool *pgxpool.Pool) *DBSink {
	return &DBSink{pool: pool}
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
	if s == nil || s.pool == nil {
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

	var (
		costUSD     *float64
		promptT     *int
		completionT *int
		latencyMs   *int64
	)
	if obs.Cost != nil {
		v := obs.Cost.CostUSD
		costUSD = &v
		if obs.Cost.PromptTokens > 0 {
			p := obs.Cost.PromptTokens
			promptT = &p
		}
		if obs.Cost.CompletionTokens > 0 {
			c := obs.Cost.CompletionTokens
			completionT = &c
		}
		if obs.Cost.LatencyMs > 0 {
			l := obs.Cost.LatencyMs
			latencyMs = &l
		}
	}

	createdAt := obs.Timestamp
	if createdAt.IsZero() {
		// pgx will default to NOW() if we pass nil.
	}
	var tsArg any = nil
	if !createdAt.IsZero() {
		tsArg = createdAt
	}

	_, err = s.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.observations
		(trace_id, session_id, domain_id, step_id, agent_id, kind, payload,
		 cost_usd, prompt_tokens, completion_tokens, latency_ms, created_at)
		VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE($12, NOW()))
	`, schema),
		nullable(obs.TraceID),
		obs.SessionID,
		nullable(obs.DomainID),
		nullable(obs.StepID),
		nullable(obs.AgentID),
		obs.Kind,
		body,
		costUSD,
		promptT,
		completionT,
		latencyMs,
		tsArg,
	)
	if err != nil {
		return fmt.Errorf("db sink: insert: %w", err)
	}
	return nil
}

// Flush is a no-op for now.
func (s *DBSink) Flush(_ context.Context) error { return nil }

// Close is a no-op; the pool's lifecycle is owned by the host server.
func (s *DBSink) Close(_ context.Context) error { return nil }

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
