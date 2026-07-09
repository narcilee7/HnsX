package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

// DefaultTenantUUID is the placeholder tenant used until the service layer
// propagates real tenant context.
const DefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists Session aggregates to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed session repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(s *model.Session) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("session/postgres: no database configured")
	}
	if s == nil || s.ID == "" {
		return model.ErrInvalidSession
	}

	ctx := context.Background()

	domainUUID, err := r.lookupDomainUUID(ctx, s.DomainID)
	if err != nil {
		return err
	}

	triggerJSON, err := json.Marshal(s.Trigger)
	if err != nil {
		return err
	}
	var resultJSON []byte
	if s.Result != nil {
		resultJSON, err = json.Marshal(s.Result)
		if err != nil {
			return err
		}
	}

	var completedAt *time.Time
	if s.CompletedAt != nil {
		completedAt = s.CompletedAt
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO sessions (
			tenant_id, session_id, domain_uuid, domain_version, orchestration, state,
			trigger_payload, result_payload, started_at, completed_at, updated_at
		)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, NOW())
		ON CONFLICT (tenant_id, session_id) DO UPDATE
		SET state = EXCLUDED.state,
		    result_payload = EXCLUDED.result_payload,
		    completed_at = EXCLUDED.completed_at,
		    updated_at = EXCLUDED.updated_at
	`, DefaultTenantUUID, s.ID, domainUUID, s.DomainVersion, s.Orchestration, string(s.State),
		string(triggerJSON), string(resultJSON), s.StartedAt, completedAt)
	return err
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.Session, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrSessionNotFound
	}

	ctx := context.Background()
	row := r.db.Pool.QueryRow(ctx, `
		SELECT s.session_id, d.domain_id, s.domain_version, s.orchestration, s.state,
		       s.trigger_payload, s.result_payload, s.started_at, s.completed_at
		FROM sessions s
		JOIN domains d ON d.id = s.domain_uuid
		WHERE s.tenant_id = $1::uuid AND s.session_id = $2
	`, DefaultTenantUUID, id)

	return r.scanSession(row)
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.Session, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT s.session_id, d.domain_id, s.domain_version, s.orchestration, s.state,
		       s.trigger_payload, s.result_payload, s.started_at, s.completed_at
		FROM sessions s
		JOIN domains d ON d.id = s.domain_uuid
		WHERE s.tenant_id = $1::uuid
	`, DefaultTenantUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Session
	for rows.Next() {
		sess, err := r.scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) ([]*model.Session, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT s.session_id, d.domain_id, s.domain_version, s.orchestration, s.state,
		       s.trigger_payload, s.result_payload, s.started_at, s.completed_at
		FROM sessions s
		JOIN domains d ON d.id = s.domain_uuid
		WHERE s.tenant_id = $1::uuid AND d.domain_id = $2
	`, DefaultTenantUUID, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Session
	for rows.Next() {
		sess, err := r.scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil || r.db.IsNoDB() {
		return nil
	}

	ctx := context.Background()
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE tenant_id = $1::uuid AND session_id = $2
	`, DefaultTenantUUID, id)
	return err
}

func (r *PostgresRepository) lookupDomainUUID(ctx context.Context, domainID string) (string, error) {
	var domainUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
		LIMIT 1
	`, DefaultTenantUUID, domainID).Scan(&domainUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", model.ErrInvalidSession
		}
		return "", err
	}
	return domainUUID, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *PostgresRepository) scanSession(row scanner) (*model.Session, error) {
	var id, domainID, domainVersion, orchestration, stateStr string
	var triggerJSON, resultJSON []byte
	var startedAt time.Time
	var completedAt *time.Time

	err := row.Scan(
		&id, &domainID, &domainVersion, &orchestration, &stateStr,
		&triggerJSON, &resultJSON, &startedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrSessionNotFound
		}
		return nil, err
	}

	var trigger map[string]any
	if len(triggerJSON) > 0 {
		if err := json.Unmarshal(triggerJSON, &trigger); err != nil {
			return nil, err
		}
	}

	var result *runtime.Result
	if len(resultJSON) > 0 {
		result = &runtime.Result{}
		if err := json.Unmarshal(resultJSON, result); err != nil {
			return nil, err
		}
	}

	return &model.Session{
		ID:            id,
		DomainID:      domainID,
		DomainVersion: domainVersion,
		Orchestration: orchestration,
		State:         model.State(stateStr),
		Trigger:       trigger,
		Result:        result,
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
	}, nil
}
