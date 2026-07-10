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

var _ Repository = (*PostgresRepository)(nil)
