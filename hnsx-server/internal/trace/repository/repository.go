// Package repository defines the trace.Repository contract and its
// in-memory implementation.
package repository

import (
	"errors"
	"sort"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
)

// Repository is the persistence contract for observation records.
type Repository interface {
	Save(record *model.ObservationRecord) error
	BySession(sessionID string) ([]model.ObservationRecord, error)
	ByTrace(traceID string) ([]model.ObservationRecord, error)
	// Aggregate returns rolled-up cost/token/invocation counts for the given
	// session IDs. Passing an empty slice returns zeroes.
	Aggregate(sessionIDs []string) (model.Aggregate, error)
	// AggregateBySession returns a per-session rollup keyed by session ID.
	// Sessions with no observations are omitted from the map. Passing an empty
	// slice returns an empty map.
	AggregateBySession(sessionIDs []string) (map[string]model.Aggregate, error)
	// ListSummaries returns a page of trace summaries that match filter. The
	// filter is the single source of truth for List; zero-valued times/strings
	// are no-ops. The second return value is the total count of matching
	// trace_ids, independent of the page size.
	ListSummaries(filter model.TraceListFilter) (model.TraceSummaryWithCount, error)
	// Detail returns the full trace, or (nil, model.ErrTraceNotFound) if no
	// observation carries the supplied trace_id.
	Detail(traceID string) (*model.TraceDetail, error)
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu       sync.RWMutex
	records  []model.ObservationRecord
	nextID   int64
}

// NewInMemoryRepository constructs an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{records: []model.ObservationRecord{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(record *model.ObservationRecord) error {
	if record == nil {
		return errors.New("trace: nil record")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	record.ID = r.nextID
	r.records = append(r.records, *record)
	return nil
}

// BySession implements Repository.
func (r *InMemoryRepository) BySession(sessionID string) ([]model.ObservationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ObservationRecord, 0)
	for _, rec := range r.records {
		if rec.SessionID == sessionID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ByTrace implements Repository.
func (r *InMemoryRepository) ByTrace(traceID string) ([]model.ObservationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ObservationRecord, 0)
	for _, rec := range r.records {
		if rec.TraceID == traceID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// Aggregate implements Repository.
func (r *InMemoryRepository) Aggregate(sessionIDs []string) (model.Aggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	want := make(map[string]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		want[id] = struct{}{}
	}
	var agg model.Aggregate
	for _, rec := range r.records {
		if len(want) > 0 {
			if _, ok := want[rec.SessionID]; !ok {
				continue
			}
		}
		agg.TotalCostUSD += rec.CostUSD
		agg.TotalPromptTokens += rec.PromptTokens
		agg.TotalCompletionTokens += rec.CompletionTokens
		switch rec.Kind {
		case "agent_invoke":
			agg.AgentInvocations++
		case "tool_call":
			agg.ToolInvocations++
		}
	}
	return agg, nil
}

// AggregateBySession implements Repository.
func (r *InMemoryRepository) AggregateBySession(sessionIDs []string) (map[string]model.Aggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	want := make(map[string]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		want[id] = struct{}{}
	}
	out := make(map[string]model.Aggregate)
	for _, rec := range r.records {
		if len(want) > 0 {
			if _, ok := want[rec.SessionID]; !ok {
				continue
			}
		}
		agg := out[rec.SessionID]
		agg.TotalCostUSD += rec.CostUSD
		agg.TotalPromptTokens += rec.PromptTokens
		agg.TotalCompletionTokens += rec.CompletionTokens
		switch rec.Kind {
		case "agent_invoke":
			agg.AgentInvocations++
		case "tool_call":
			agg.ToolInvocations++
		}
		out[rec.SessionID] = agg
	}
	return out, nil
}

var _ Repository = (*InMemoryRepository)(nil)

// ListSummaries implements Repository.
func (r *InMemoryRepository) ListSummaries(filter model.TraceListFilter) (model.TraceSummaryWithCount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byTrace := make(map[string]*model.TraceSummary)
	for _, rec := range r.records {
		if rec.TraceID == "" {
			continue
		}
		if filter.DomainID != "" && rec.DomainID != filter.DomainID {
			continue
		}
		if filter.SessionID != "" && rec.SessionID != filter.SessionID {
			continue
		}
		if filter.AgentID != "" && rec.AgentID != filter.AgentID {
			continue
		}
		if !filter.From.IsZero() && rec.CreatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && rec.CreatedAt.After(filter.To) {
			continue
		}
		sum, ok := byTrace[rec.TraceID]
		if !ok {
			sum = &model.TraceSummary{
				TraceID:       rec.TraceID,
				SessionID:     rec.SessionID,
				DomainID:      rec.DomainID,
				DomainVersion: rec.DomainVersion,
				Status:        "running",
			}
			byTrace[rec.TraceID] = sum
		}
		if rec.CreatedAt.Before(sum.StartedAt) || sum.StartedAt.IsZero() {
			sum.StartedAt = rec.CreatedAt
		}
		if rec.CreatedAt.After(sum.CompletedAt) {
			sum.CompletedAt = rec.CreatedAt
		}
		sum.ObservationCount++
		sum.TotalCostUSD += rec.CostUSD
		sum.TotalPromptTokens += rec.PromptTokens
		sum.TotalCompletionTokens += rec.CompletionTokens
		switch rec.Kind {
		case "agent_invoke":
			sum.AgentInvocations++
		case "tool_call":
			sum.ToolInvocations++
		}
		if rec.Kind == "session_end" {
			// Last session_end wins; the loop processes records in insertion order,
			// but CompletedAt already tracks the latest observation so status
			// collapses to "completed" once any final observation is seen.
			sum.Status = "completed"
		}
	}

	summaries := make([]model.TraceSummary, 0, len(byTrace))
	for _, sum := range byTrace {
		if !sum.CompletedAt.IsZero() && !sum.StartedAt.IsZero() {
			sum.DurationMs = sum.CompletedAt.Sub(sum.StartedAt).Milliseconds()
		}
		summaries = append(summaries, *sum)
	}
	// Stable ordering: most recent traces first.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartedAt.After(summaries[j].StartedAt)
	})

	total := len(summaries)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return model.TraceSummaryWithCount{
		Summaries: summaries[offset:end],
		Total:     total,
	}, nil
}

// Detail implements Repository.
func (r *InMemoryRepository) Detail(traceID string) (*model.TraceDetail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matched := make([]model.ObservationRecord, 0)
	for _, rec := range r.records {
		if rec.TraceID == traceID {
			matched = append(matched, rec)
		}
	}
	if len(matched) == 0 {
		return nil, model.ErrTraceNotFound
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.Before(matched[j].CreatedAt)
	})

	sum := model.TraceSummary{
		TraceID:       traceID,
		SessionID:     matched[0].SessionID,
		DomainID:      matched[0].DomainID,
		DomainVersion: matched[0].DomainVersion,
		Status:        "running",
		StartedAt:     matched[0].CreatedAt,
		CompletedAt:   matched[len(matched)-1].CreatedAt,
	}
	for _, rec := range matched {
		sum.ObservationCount++
		sum.TotalCostUSD += rec.CostUSD
		sum.TotalPromptTokens += rec.PromptTokens
		sum.TotalCompletionTokens += rec.CompletionTokens
		switch rec.Kind {
		case "agent_invoke":
			sum.AgentInvocations++
		case "tool_call":
			sum.ToolInvocations++
		}
		if rec.Kind == "session_end" {
			sum.Status = "completed"
		}
	}
	if !sum.CompletedAt.IsZero() && !sum.StartedAt.IsZero() {
		sum.DurationMs = sum.CompletedAt.Sub(sum.StartedAt).Milliseconds()
	}
	return &model.TraceDetail{
		TraceSummary: sum,
		Observations: matched,
	}, nil
}
