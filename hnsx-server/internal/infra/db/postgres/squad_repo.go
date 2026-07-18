package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
	"gorm.io/gorm"
)

// ErrSquadNotFound is returned when a squad lookup misses.
var ErrSquadNotFound = errors.New("squad: not found")

// SquadRepo implements squad.Repo against Postgres via GORM.
//
// Members are stored as a JSONB column on the squads row. R1.4 keeps it
// inline for simplicity; if querying "squads containing member X" becomes
// hot, R3+ can split into a squad_members table.
type SquadRepo struct{ db *DB }

func NewSquadRepo(db *DB) *SquadRepo { return &SquadRepo{db: db} }

func (r *SquadRepo) Create(ctx context.Context, s *squad.Squad) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *SquadRepo) Get(ctx context.Context, id string) (*squad.Squad, error) {
	var s squad.Squad
	err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSquadNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SquadRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]*squad.Squad, error) {
	var out []*squad.Squad
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SquadRepo) Update(ctx context.Context, s *squad.Squad) error {
	if err := s.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&squad.Squad{}).
		Where("id = ?", s.ID).
		Updates(map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"members":     s.Members,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSquadNotFound
	}
	return nil
}

func (r *SquadRepo) AddMember(ctx context.Context, squadID string, m squad.Member) error {
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now().UTC()
	}
	// GORM's JSONB updates via Updates() with a map merge the new member
	// into the existing array. We do an in-memory merge to ensure the
	// operation is idempotent.
	var s squad.Squad
	if err := r.db.WithContext(ctx).First(&s, "id = ?", squadID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSquadNotFound
		}
		return err
	}
	for _, existing := range s.Members {
		if existing.ID == m.ID {
			return nil // already a member — idempotent
		}
	}
	s.Members = append(s.Members, m)
	res := r.db.WithContext(ctx).
		Model(&squad.Squad{}).
		Where("id = ?", squadID).
		Update("members", s.Members)
	return res.Error
}

func (r *SquadRepo) RemoveMember(ctx context.Context, squadID, memberID string) error {
	var s squad.Squad
	if err := r.db.WithContext(ctx).First(&s, "id = ?", squadID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSquadNotFound
		}
		return err
	}
	out := s.Members[:0]
	for _, m := range s.Members {
		if m.ID != memberID {
			out = append(out, m)
		}
	}
	if len(out) == len(s.Members) {
		return nil // no-op; idempotent
	}
	s.Members = out
	res := r.db.WithContext(ctx).
		Model(&squad.Squad{}).
		Where("id = ?", squadID).
		Update("members", s.Members)
	return res.Error
}

func (r *SquadRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&squad.Squad{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSquadNotFound
	}
	return nil
}

var _ squad.Repo = (*SquadRepo)(nil)