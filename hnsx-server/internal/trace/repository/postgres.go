package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

// PostgresRepository persists observation records to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed trace repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(record *model.ObservationRecord) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("trace/postgres: no database configured")
	}
	if record == nil {
		return model.ErrTraceNotFound
	}

	payloadJSON, err := json.Marshal(record.Payload)
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO observations (
			trace_id, session_id, domain_id, domain_version, step_id,
			agent_id, kind, payload, cost_usd, prompt_tokens, completion_tokens, latency_ms, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13)
	`, record.TraceID, record.SessionID, record.DomainID, record.DomainVersion, record.StepID,
		record.AgentID, record.Kind, string(payloadJSON), record.CostUSD, record.PromptTokens,
		record.CompletionTokens, record.LatencyMs, record.CreatedAt)
	return err
}

// BySession implements Repository.
func (r *PostgresRepository) BySession(sessionID string) ([]model.ObservationRecord, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT trace_id, session_id, domain_id, domain_version, step_id, agent_id, kind,
		       payload, cost_usd, prompt_tokens, completion_tokens, latency_ms, created_at
		FROM observations
		WHERE session_id = $1
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRecords(rows)
}

// ByTrace implements Repository.
func (r *PostgresRepository) ByTrace(traceID string) ([]model.ObservationRecord, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT trace_id, session_id, domain_id, domain_version, step_id, agent_id, kind,
		       payload, cost_usd, prompt_tokens, completion_tokens, latency_ms, created_at
		FROM observations
		WHERE trace_id = $1
		ORDER BY created_at ASC
	`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRecords(rows)
}

func (r *PostgresRepository) scanRecords(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]model.ObservationRecord, error) {
	var out []model.ObservationRecord
	for rows.Next() {
		var rec model.ObservationRecord
		var payloadJSON []byte
		if err := rows.Scan(
			&rec.TraceID, &rec.SessionID, &rec.DomainID, &rec.DomainVersion, &rec.StepID,
			&rec.AgentID, &rec.Kind, &payloadJSON, &rec.CostUSD, &rec.PromptTokens,
			&rec.CompletionTokens, &rec.LatencyMs, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if len(payloadJSON) > 0 {
			_ = json.Unmarshal(payloadJSON, &rec.Payload)
		}
		if rec.Payload == nil {
			rec.Payload = map[string]any{}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

var _ Repository = (*PostgresRepository)(nil)
