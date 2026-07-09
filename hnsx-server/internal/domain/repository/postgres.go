package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// DefaultTenantUUID is the placeholder tenant used until the service layer
// propagates real tenant context.
const DefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists RegisteredDomain aggregates to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed domain repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(d *model.RegisteredDomain) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("domain/postgres: no database configured")
	}
	if err := d.Validate(); err != nil {
		return err
	}

	specJSON, err := json.Marshal(d.Spec)
	if err != nil {
		return err
	}

	ctx := context.Background()
	now := time.Now().UTC()

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Upsert the domain registry row.
	var domainUUID string
	err = tx.QueryRow(ctx, `
		INSERT INTO domains (tenant_id, domain_id, current_version, description, status, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, 'active', $5, $6)
		ON CONFLICT (tenant_id, domain_id) DO UPDATE
		SET current_version = EXCLUDED.current_version,
		    description = EXCLUDED.description,
		    updated_at = EXCLUDED.updated_at
		RETURNING id
	`, DefaultTenantUUID, d.ID, d.Version, d.Description, now, now).Scan(&domainUUID)
	if err != nil {
		return err
	}

	// Insert a new version row. Older versions are retained for history.
	_, err = tx.Exec(ctx, `
		INSERT INTO domain_versions (tenant_id, domain_uuid, version, yaml_body, json_body, harness_hash, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (tenant_id, domain_uuid, version) DO UPDATE
		SET yaml_body = EXCLUDED.yaml_body,
		    json_body = EXCLUDED.json_body,
		    harness_hash = EXCLUDED.harness_hash
	`, DefaultTenantUUID, domainUUID, d.Version, string(specJSON), string(specJSON), "", now)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.RegisteredDomain, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrDomainNotFound
	}

	ctx := context.Background()
	row := r.db.Pool.QueryRow(ctx, `
		SELECT d.id, d.current_version, d.description, d.created_at, d.updated_at, dv.json_body
		FROM domains d
		JOIN domain_versions dv ON dv.domain_uuid = d.id AND dv.version = d.current_version
		WHERE d.tenant_id = $1::uuid AND d.domain_id = $2
		ORDER BY dv.created_at DESC
		LIMIT 1
	`, DefaultTenantUUID, id)

	var domainUUID, version, description string
	var createdAt, updatedAt time.Time
	var specJSON []byte
	err := row.Scan(&domainUUID, &version, &description, &createdAt, &updatedAt, &specJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	var spec spec.DomainSpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, err
	}

	return &model.RegisteredDomain{
		ID:          id,
		Version:     version,
		Description: description,
		Spec:        &spec,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.RegisteredDomain, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT d.domain_id, d.current_version, d.description, d.created_at, d.updated_at, dv.json_body
		FROM domains d
		JOIN domain_versions dv ON dv.domain_uuid = d.id AND dv.version = d.current_version
		WHERE d.tenant_id = $1::uuid
	`, DefaultTenantUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.RegisteredDomain
	for rows.Next() {
		var id, version, description string
		var createdAt, updatedAt time.Time
		var specJSON []byte
		if err := rows.Scan(&id, &version, &description, &createdAt, &updatedAt, &specJSON); err != nil {
			return nil, err
		}
		var spec spec.DomainSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return nil, err
		}
		out = append(out, &model.RegisteredDomain{
			ID:          id,
			Version:     version,
			Description: description,
			Spec:        &spec,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		})
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
		DELETE FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
	`, DefaultTenantUUID, id)
	return err
}

// Exists implements Repository.
func (r *PostgresRepository) Exists(id string) (bool, error) {
	if r.db == nil || r.db.IsNoDB() {
		return false, nil
	}

	ctx := context.Background()
	var one int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT 1 FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
		LIMIT 1
	`, DefaultTenantUUID, id).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var _ Repository = (*PostgresRepository)(nil)
