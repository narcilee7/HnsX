package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const secretDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists secrets to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed secret repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(s *model.Secret) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("secret/postgres: no database configured")
	}
	if s == nil || s.Name == "" {
		return model.ErrInvalidName
	}
	if s.Kind == "" {
		s.Kind = "generic"
	}

	ctx := context.Background()
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO secrets (tenant_id, secret_id, value, description, kind, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, NOW())
		ON CONFLICT (tenant_id, secret_id) DO UPDATE
		SET value = EXCLUDED.value,
		    description = EXCLUDED.description,
		    kind = EXCLUDED.kind,
		    updated_at = NOW()
	`, secretDefaultTenantUUID, s.Name, s.Value, "", s.Kind)
	return err
}

// ByName implements Repository.
func (r *PostgresRepository) ByName(name string) (*model.Secret, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrSecretNotFound
	}

	ctx := context.Background()
	var s model.Secret
	err := r.db.Pool.QueryRow(ctx, `
		SELECT secret_id, value, kind
		FROM secrets
		WHERE tenant_id = $1::uuid AND secret_id = $2
		LIMIT 1
	`, secretDefaultTenantUUID, name).Scan(&s.Name, &s.Value, &s.Kind)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrSecretNotFound
		}
		return nil, err
	}
	return &s, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(name string) error {
	if r.db == nil || r.db.IsNoDB() {
		return nil
	}

	ctx := context.Background()
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM secrets
		WHERE tenant_id = $1::uuid AND secret_id = $2
	`, secretDefaultTenantUUID, name)
	return err
}

var _ Repository = (*PostgresRepository)(nil)
