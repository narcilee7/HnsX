package repository

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

// PostgresRepository persists observation records to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed trace repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(record *model.ObservationRecord) error {
	if r.db == nil {
		return errors.New("trace/postgres: no database configured")
	}
	if record == nil {
		return model.ErrTraceNotFound
	}

	payloadJSON, err := json.Marshal(record.Payload)
	if err != nil {
		return err
	}

	entity := ObservationRecord{
		TraceID:          record.TraceID,
		SessionID:        record.SessionID,
		DomainID:         record.DomainID,
		DomainVersion:    record.DomainVersion,
		StepID:           record.StepID,
		AgentID:          record.AgentID,
		Kind:             record.Kind,
		Payload:          payloadJSON,
		CostUSD:          record.CostUSD,
		PromptTokens:     record.PromptTokens,
		CompletionTokens: record.CompletionTokens,
		LatencyMs:        record.LatencyMs,
		CreatedAt:        record.CreatedAt,
	}

	return r.db.Create(&entity).Error
}

// BySession implements Repository.
func (r *PostgresRepository) BySession(sessionID string) ([]model.ObservationRecord, error) {
	if r.db == nil {
		return nil, nil
	}

	var records []ObservationRecord
	if err := r.db.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	return r.toModels(records), nil
}

// ByTrace implements Repository.
func (r *PostgresRepository) ByTrace(traceID string) ([]model.ObservationRecord, error) {
	if r.db == nil {
		return nil, nil
	}

	var records []ObservationRecord
	if err := r.db.Where("trace_id = ?", traceID).Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	return r.toModels(records), nil
}

func (r *PostgresRepository) toModels(records []ObservationRecord) []model.ObservationRecord {
	out := make([]model.ObservationRecord, 0, len(records))
	for _, rec := range records {
		out = append(out, r.toModel(rec))
	}
	return out
}

func (r *PostgresRepository) toModel(rec ObservationRecord) model.ObservationRecord {
	payload := map[string]any{}
	if len(rec.Payload) > 0 {
		_ = json.Unmarshal(rec.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}

	return model.ObservationRecord{
		ID:               rec.ID,
		TraceID:          rec.TraceID,
		SessionID:        rec.SessionID,
		DomainID:         rec.DomainID,
		DomainVersion:    rec.DomainVersion,
		StepID:           rec.StepID,
		AgentID:          rec.AgentID,
		Kind:             rec.Kind,
		Payload:          payload,
		CostUSD:          rec.CostUSD,
		PromptTokens:     rec.PromptTokens,
		CompletionTokens: rec.CompletionTokens,
		LatencyMs:        rec.LatencyMs,
		CreatedAt:        rec.CreatedAt,
	}
}

var _ Repository = (*PostgresRepository)(nil)
