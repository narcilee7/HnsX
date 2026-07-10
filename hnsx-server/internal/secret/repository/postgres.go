package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const secretDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists secrets to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed secret repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(s *model.Secret) error {
	if r.db == nil {
		return errors.New("secret/postgres: no database configured")
	}
	if s == nil || s.Name == "" {
		return model.ErrInvalidName
	}
	if s.Kind == "" {
		s.Kind = "generic"
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec SecretRecord
		err := tx.Where("tenant_id = ? AND secret_id = ?", secretDefaultTenantUUID, s.Name).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = secretDefaultTenantUUID
		rec.SecretID = s.Name
		rec.Value = s.Value
		rec.Kind = s.Kind

		if isNew {
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

// ByName implements Repository.
func (r *PostgresRepository) ByName(name string) (*model.Secret, error) {
	if r.db == nil {
		return nil, model.ErrSecretNotFound
	}

	var rec SecretRecord
	if err := r.db.Where("tenant_id = ? AND secret_id = ?", secretDefaultTenantUUID, name).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrSecretNotFound
		}
		return nil, err
	}

	return &model.Secret{
		ID:    rec.ID,
		Name:  rec.SecretID,
		Value: rec.Value,
		Kind:  rec.Kind,
	}, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(name string) error {
	if r.db == nil {
		return nil
	}

	return r.db.Where("tenant_id = ? AND secret_id = ?", secretDefaultTenantUUID, name).
		Delete(&SecretRecord{}).Error
}

// List implements Repository. It returns metadata only (no Value);
// callers who need the plaintext must go through Service.Resolve so the
// access is audit-attributed.
func (r *PostgresRepository) List() ([]model.ListItem, error) {
	out := []model.ListItem{}
	if r.db == nil {
		return out, nil
	}
	var rows []SecretRecord
	if err := r.db.Where("tenant_id = ?", secretDefaultTenantUUID).
		Order("secret_id ASC").
		Find(&rows).Error; err != nil {
		return out, err
	}
	for _, row := range rows {
		// Compute fingerprint from the stored envelope so it matches what
		// an operator sees on the wire against what came back in.
		fingerprint := ""
		if row.Value != "" {
			if n := len(row.Value); n > 4 {
				fingerprint = "****" + row.Value[n-4:]
			}
		}
		out = append(out, model.ListItem{
			Name:        row.SecretID,
			Description: row.Description,
			Kind:        row.Kind,
			Fingerprint: fingerprint,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	return out, nil
}

var _ Repository = (*PostgresRepository)(nil)
