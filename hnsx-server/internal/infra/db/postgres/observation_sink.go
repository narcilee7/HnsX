package postgres

import (
	"context"

	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
)

// observation.ErrObservationNotFound is returned when an observation lookup misses.

// ObservationSink implements observation.Sink against Postgres via GORM.
type ObservationSink struct{ db *DB }

func NewObservationSink(db *DB) *ObservationSink { return &ObservationSink{db: db} }

func (s *ObservationSink) Write(ctx context.Context, obs *observation.Observation) error {
	return s.db.WithContext(ctx).Create(obs).Error
}

func (s *ObservationSink) ListByIssue(ctx context.Context, issueID string, limit int) ([]*observation.Observation, error) {
	if limit <= 0 {
		limit = 200
	}
	var out []*observation.Observation
	err := s.db.WithContext(ctx).
		Where("issue_id = ?", issueID).
		Order("occurred_at ASC, sequence ASC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ObservationSink) ListByEvalRun(ctx context.Context, evalRunID string) ([]*observation.Observation, error) {
	var out []*observation.Observation
	err := s.db.WithContext(ctx).
		Where("eval_run_id = ?", evalRunID).
		Order("occurred_at ASC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

var _ observation.Sink = (*ObservationSink)(nil)