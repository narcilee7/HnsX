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
		return errors.New("trace/postgres: unsupported record")
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

// Aggregate implements Repository.
func (r *PostgresRepository) Aggregate(sessionIDs []string) (model.Aggregate, error) {
	var agg model.Aggregate
	if r.db == nil {
		return agg, nil
	}

	var result = new(model.Aggregate)

	query := r.db.Model(&ObservationRecord{}).
		Select(`COALESCE(SUM(cost_usd), 0) AS cost_usd,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COUNT(*) FILTER (WHERE kind = ?) AS agent_invocations,
			COUNT(*) FILTER (WHERE kind = ?) AS tool_invocations`,
			"agent_invoke", "tool_call")
	if len(sessionIDs) > 0 {
		query = query.Where("session_id IN ?", sessionIDs)
	}
	if err := query.Scan(&result).Error; err != nil {
		return agg, err
	}

	agg.TotalCostUSD = result.TotalCostUSD
	agg.TotalPromptTokens = result.TotalPromptTokens
	agg.TotalCompletionTokens = result.TotalCompletionTokens
	agg.AgentInvocations = int(result.AgentInvocations)
	agg.ToolInvocations = int(result.ToolInvocations)
	return agg, nil
}

// AggregateBySession implements Repository.
func (r *PostgresRepository) AggregateBySession(sessionIDs []string) (map[string]model.Aggregate, error) {
	out := make(map[string]model.Aggregate)
	if r.db == nil {
		return out, nil
	}

	var rows = make([]model.AggregateWithSession, 0, len(sessionIDs))

	query := r.db.Model(&ObservationRecord{}).
		Select(`session_id,
			COALESCE(SUM(cost_usd), 0) AS cost_usd,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COUNT(*) FILTER (WHERE kind = ?) AS agent_invocations,
			COUNT(*) FILTER (WHERE kind = ?) AS tool_invocations`,
			"agent_invoke", "tool_call").
		Group("session_id")
	if len(sessionIDs) > 0 {
		query = query.Where("session_id IN ?", sessionIDs)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return out, err
	}

	for _, row := range rows {
		out[row.SessionID] = model.Aggregate{
			TotalCostUSD:          row.TotalCostUSD,
			TotalPromptTokens:     row.TotalPromptTokens,
			TotalCompletionTokens: row.TotalCompletionTokens,
			AgentInvocations:      int(row.AgentInvocations),
			ToolInvocations:       int(row.ToolInvocations),
		}
	}
	return out, nil
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
