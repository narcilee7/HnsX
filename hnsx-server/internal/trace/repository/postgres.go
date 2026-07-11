package repository

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

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
	metadataJSON, err := json.Marshal(record.Metadata)
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
		Metadata:         metadataJSON,
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

// ListSummaries implements Repository. It groups observations by trace_id
// server-side and emits one TraceSummary row per group. The (trace_id)
// and (domain_id, created_at) indexes are used; the LIMIT/OFFSET columns
// are applied to the OUTER query so the row count reflects the page, not
// the underlying observation count.
func (r *PostgresRepository) ListSummaries(filter model.TraceListFilter) (model.TraceSummaryWithCount, error) {
	out := model.TraceSummaryWithCount{}
	if r.db == nil {
		return out, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Inner aggregation: one row per (trace_id, session_id, domain_id,
	// domain_version) carrying the per-trace rollups + earliest/latest
	// observation timestamps. We use the latest observation's status-ish
	// hint (session_end marks completed) via a separate subquery in
	// Detail; for List we keep the rollup simple and rely on the most
	// recent observation as the completion marker.
	type aggRow struct {
		TraceID               string
		SessionID             string
		DomainID              string
		DomainVersion         string
		StartedAt             time.Time
		CompletedAt           time.Time
		ObservationCount      int
		TotalCostUSD          float64
		TotalPromptTokens     int
		TotalCompletionTokens int
		AgentInvocations      int
		ToolInvocations       int
		HadSessionEnd         int
	}

	var rows []aggRow
	q := r.db.Model(&ObservationRecord{}).
		Select(`trace_id AS trace_id,
			MIN(session_id) AS session_id,
			MIN(domain_id) AS domain_id,
			MIN(domain_version) AS domain_version,
			MIN(created_at) AS started_at,
			MAX(created_at) AS completed_at,
			COUNT(*) AS observation_count,
			COALESCE(SUM(cost_usd), 0) AS total_cost_usd,
			COALESCE(SUM(prompt_tokens), 0) AS total_prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS total_completion_tokens,
			COUNT(*) FILTER (WHERE kind = ?) AS agent_invocations,
			COUNT(*) FILTER (WHERE kind = ?) AS tool_invocations,
			COUNT(*) FILTER (WHERE kind = ?) AS had_session_end`,
			"agent_invoke", "tool_call", "session_end").
		Where("trace_id <> ''").
		Group("trace_id")

	q = applyTraceFilter(q, filter)

	if err := q.Session(&gorm.Session{}).Scan(&rows).Error; err != nil {
		return out, err
	}

	summaries := make([]model.TraceSummary, 0, len(rows))
	for _, row := range rows {
		sum := model.TraceSummary{
			TraceID:               row.TraceID,
			SessionID:             row.SessionID,
			DomainID:              row.DomainID,
			DomainVersion:         row.DomainVersion,
			StartedAt:             row.StartedAt,
			CompletedAt:           row.CompletedAt,
			ObservationCount:      row.ObservationCount,
			TotalCostUSD:          row.TotalCostUSD,
			TotalPromptTokens:     row.TotalPromptTokens,
			TotalCompletionTokens: row.TotalCompletionTokens,
			AgentInvocations:      row.AgentInvocations,
			ToolInvocations:       row.ToolInvocations,
			Status:                "running",
		}
		if row.HadSessionEnd > 0 {
			sum.Status = "completed"
		}
		if !sum.CompletedAt.IsZero() && !sum.StartedAt.IsZero() {
			sum.DurationMs = sum.CompletedAt.Sub(sum.StartedAt).Milliseconds()
		}
		summaries = append(summaries, sum)
	}

	// Stable order: most recent first by started_at desc, then trace_id
	// asc as a deterministic tie-breaker.
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].StartedAt.Equal(summaries[j].StartedAt) {
			return summaries[i].TraceID < summaries[j].TraceID
		}
		return summaries[i].StartedAt.After(summaries[j].StartedAt)
	})

	total := len(summaries)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out.Summaries = summaries[offset:end]
	out.Total = total
	return out, nil
}

// Detail implements Repository. It loads the observations for trace_id in
// chronological order, then computes the same per-trace rollup as
// ListSummaries so the two endpoints stay symmetric.
func (r *PostgresRepository) Detail(traceID string) (*model.TraceDetail, error) {
	if r.db == nil {
		return nil, model.ErrTraceNotFound
	}
	var records []ObservationRecord
	if err := r.db.Where("trace_id = ?", traceID).
		Order("created_at ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, model.ErrTraceNotFound
	}

	sum := model.TraceSummary{
		TraceID:       traceID,
		SessionID:     records[0].SessionID,
		DomainID:      records[0].DomainID,
		DomainVersion: records[0].DomainVersion,
		Status:        "running",
		StartedAt:     records[0].CreatedAt,
		CompletedAt:   records[len(records)-1].CreatedAt,
	}
	for _, rec := range records {
		sum.ObservationCount++
		sum.TotalCostUSD += rec.CostUSD
		sum.TotalPromptTokens += rec.PromptTokens
		sum.TotalCompletionTokens += rec.CompletionTokens
		switch rec.Kind {
		case "agent_invoke":
			sum.AgentInvocations++
		case "tool_call":
			sum.ToolInvocations++
		case "session_end":
			sum.Status = "completed"
		}
	}
	if !sum.CompletedAt.IsZero() && !sum.StartedAt.IsZero() {
		sum.DurationMs = sum.CompletedAt.Sub(sum.StartedAt).Milliseconds()
	}

	return &model.TraceDetail{
		TraceSummary: sum,
		Observations: r.toModels(records),
	}, nil
}

func applyTraceFilter(q *gorm.DB, filter model.TraceListFilter) *gorm.DB {
	if filter.DomainID != "" {
		q = q.Where("domain_id = ?", filter.DomainID)
	}
	if filter.SessionID != "" {
		q = q.Where("session_id = ?", filter.SessionID)
	}
	if filter.AgentID != "" {
		q = q.Where("agent_id = ?", filter.AgentID)
	}
	if !filter.From.IsZero() {
		q = q.Where("created_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("created_at <= ?", filter.To)
	}
	return q
}

func (r *PostgresRepository) toModel(rec ObservationRecord) model.ObservationRecord {
	payload := map[string]any{}
	if len(rec.Payload) > 0 {
		_ = json.Unmarshal(rec.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	metadata := map[string]any{}
	if len(rec.Metadata) > 0 {
		_ = json.Unmarshal(rec.Metadata, &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
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
		Metadata:         metadata,
		CostUSD:          rec.CostUSD,
		PromptTokens:     rec.PromptTokens,
		CompletionTokens: rec.CompletionTokens,
		LatencyMs:        rec.LatencyMs,
		CreatedAt:        rec.CreatedAt,
	}
}

var _ Repository = (*PostgresRepository)(nil)
