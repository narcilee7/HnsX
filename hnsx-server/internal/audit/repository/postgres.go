package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const auditDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists audit entries to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed audit repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(e *model.Entry) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("audit/postgres: no database configured")
	}
	if e == nil {
		return model.ErrAuditEntryNotFound
	}
	if err := e.Validate(); err != nil {
		return err
	}

	ctx := context.Background()

	var domainUUID, sessionUUID *string
	if e.DomainID != "" {
		uuid, err := r.lookupDomainUUID(ctx, e.DomainID)
		if err == nil {
			domainUUID = &uuid
		}
	}
	if e.SessionID != "" {
		uuid, err := r.lookupSessionUUID(ctx, e.SessionID)
		if err == nil {
			sessionUUID = &uuid
		}
	}

	detailsJSON, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO audit_logs (
			tenant_id, timestamp, session_uuid, domain_uuid, action,
			actor, actor_type, resource, resource_type, decision, reason, details
		)
		VALUES ($1::uuid, $2, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12::jsonb)
	`, auditDefaultTenantUUID, e.Timestamp, sessionUUID, domainUUID, e.Action,
		e.Actor, e.ActorType, e.Resource, e.ResourceType, e.Decision, e.Reason, string(detailsJSON))
	return err
}

// BySession implements Repository.
func (r *PostgresRepository) BySession(sessionID string) ([]model.Entry, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	sessionUUID, err := r.lookupSessionUUID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT timestamp, action, actor, actor_type, resource, resource_type, decision, reason, details
		FROM audit_logs
		WHERE tenant_id = $1::uuid AND session_uuid = $2::uuid
		ORDER BY timestamp DESC
	`, auditDefaultTenantUUID, sessionUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) ([]model.Entry, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	domainUUID, err := r.lookupDomainUUID(ctx, domainID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT timestamp, action, actor, actor_type, resource, resource_type, decision, reason, details
		FROM audit_logs
		WHERE tenant_id = $1::uuid AND domain_uuid = $2::uuid
		ORDER BY timestamp DESC
	`, auditDefaultTenantUUID, domainUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// List implements Repository.
func (r *PostgresRepository) List(limit, offset int) ([]model.Entry, int, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, 0, nil
	}

	ctx := context.Background()
	var total int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs WHERE tenant_id = $1::uuid
	`, auditDefaultTenantUUID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT timestamp, action, actor, actor_type, resource, resource_type, decision, reason, details
		FROM audit_logs
		WHERE tenant_id = $1::uuid
		ORDER BY timestamp DESC
		LIMIT $2 OFFSET $3
	`, auditDefaultTenantUUID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := r.scanEntries(rows)
	return entries, total, err
}

func (r *PostgresRepository) scanEntries(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]model.Entry, error) {
	var out []model.Entry
	for rows.Next() {
		var e model.Entry
		var detailsJSON []byte
		if err := rows.Scan(&e.Timestamp, &e.Action, &e.Actor, &e.ActorType, &e.Resource, &e.ResourceType, &e.Decision, &e.Reason, &detailsJSON); err != nil {
			return nil, err
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) lookupDomainUUID(ctx context.Context, domainID string) (string, error) {
	var domainUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
		LIMIT 1
	`, auditDefaultTenantUUID, domainID).Scan(&domainUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("audit/postgres: domain not found")
		}
		return "", err
	}
	return domainUUID, nil
}

func (r *PostgresRepository) lookupSessionUUID(ctx context.Context, sessionID string) (string, error) {
	var sessionUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM sessions
		WHERE tenant_id = $1::uuid AND session_id = $2
		LIMIT 1
	`, auditDefaultTenantUUID, sessionID).Scan(&sessionUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("audit/postgres: session not found")
		}
		return "", err
	}
	return sessionUUID, nil
}

var _ Repository = (*PostgresRepository)(nil)
