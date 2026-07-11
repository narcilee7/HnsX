package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

const domainDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists RegisteredDomain aggregates to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed domain repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(d *model.RegisteredDomain) error {
	if r.db == nil {
		return errors.New("domain/postgres: no database configured")
	}
	if err := d.Validate(); err != nil {
		return err
	}

	specJSON, err := json.Marshal(d.Spec)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec DomainRecord
		err := tx.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, d.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = domainDefaultTenantUUID
		rec.DomainID = d.ID
		rec.CurrentVersion = d.Version
		rec.Description = d.Description
		rec.Status = "active"
		rec.UpdatedAt = now
		if isNew {
			rec.CreatedAt = now
		}

		if err := tx.Save(&rec).Error; err != nil {
			return err
		}

		version := DomainVersionRecord{
			TenantID:    domainDefaultTenantUUID,
			DomainUUID:  rec.ID,
			Version:     d.Version,
			YAMLBody:    string(specJSON),
			JSONBody:    specJSON,
			HarnessHash: "",
			CreatedAt:   now,
		}
		return tx.Save(&version).Error
	})
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.RegisteredDomain, error) {
	if r.db == nil {
		return nil, model.ErrDomainNotFound
	}

	var rec DomainRecord
	if err := r.db.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	return r.toModel(rec)
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.RegisteredDomain, error) {
	if r.db == nil {
		return nil, nil
	}

	var records []DomainRecord
	if err := r.db.Where("tenant_id = ?", domainDefaultTenantUUID).Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]*model.RegisteredDomain, 0, len(records))
	for _, rec := range records {
		d, err := r.toModel(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil {
		return nil
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec DomainRecord
		if err := tx.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
			Take(&rec).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		if err := tx.Where("tenant_id = ? AND domain_uuid = ?", domainDefaultTenantUUID, rec.ID).
			Delete(&DomainVersionRecord{}).Error; err != nil {
			return err
		}

		return tx.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
			Delete(&DomainRecord{}).Error
	})
}

// Exists implements Repository.
func (r *PostgresRepository) Exists(id string) (bool, error) {
	if r.db == nil {
		return false, nil
	}

	var count int64
	err := r.db.Model(&DomainRecord{}).
		Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
		Count(&count).Error
	return count > 0, err
}

// ListVersions implements Repository.
func (r *PostgresRepository) ListVersions(id string) ([]VersionRecord, error) {
	if r.db == nil {
		return nil, model.ErrDomainNotFound
	}

	var rec DomainRecord
	if err := r.db.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	var rows []DomainVersionRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", domainDefaultTenantUUID, rec.ID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]VersionRecord, len(rows))
	for i, v := range rows {
		var s spec.DomainSpec
		if err := json.Unmarshal(v.JSONBody, &s); err != nil {
			return nil, err
		}
		out[i] = VersionRecord{
			Version:   v.Version,
			CreatedAt: v.CreatedAt,
			Spec:      &s,
		}
	}
	return out, nil
}

// GetVersion implements Repository.
func (r *PostgresRepository) GetVersion(id, version string) (*spec.DomainSpec, error) {
	if r.db == nil {
		return nil, model.ErrDomainNotFound
	}

	var rec DomainRecord
	if err := r.db.Where("tenant_id = ? AND domain_id = ?", domainDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	var v DomainVersionRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ? AND version = ?", domainDefaultTenantUUID, rec.ID, version).
		Take(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	var s spec.DomainSpec
	if err := json.Unmarshal(v.JSONBody, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepository) toModel(rec DomainRecord) (*model.RegisteredDomain, error) {
	var version DomainVersionRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", domainDefaultTenantUUID, rec.ID).
		Order("created_at DESC").
		Take(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrDomainNotFound
		}
		return nil, err
	}

	var s spec.DomainSpec
	if err := json.Unmarshal(version.JSONBody, &s); err != nil {
		return nil, err
	}

	return &model.RegisteredDomain{
		ID:          rec.DomainID,
		Version:     rec.CurrentVersion,
		Description: rec.Description,
		Spec:        &s,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}, nil
}

var _ Repository = (*PostgresRepository)(nil)

// Ensure context import is referenced when needed.
var _ = context.Background()
