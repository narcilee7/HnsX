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

var _ Repository = (*InMemoryRepository)(nil)
