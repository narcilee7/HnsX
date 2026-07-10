package repository

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
	"github.com/hnsx-io/hnsx/server/internal/policy/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const policyDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists Policy aggregates to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed policy repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository. The policy_id column is the upsert key;
// domain_uuid is set when a binding is supplied.
func (r *PostgresRepository) Save(p *model.Policy) error {
	if r.db == nil {
		return errors.New("policy/postgres: no database configured")
	}
	if p == nil || p.ID == "" {
		return model.ErrInvalidPolicyID
	}

	rulesJSON, err := json.Marshal(model.Rules{
		Budget:      p.Budget,
		Permissions: p.Permissions,
		Guardrails:  p.Guardrails,
	})
	if err != nil {
		return err
	}

	var domainUUID *string
	if p.BoundDomain != "" {
		uuid, err := r.lookupDomainUUID(p.BoundDomain)
		if err != nil {
			return err
		}
		domainUUID = &uuid
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec PolicyRecord
		err := tx.Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, p.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		now := time.Now().UTC()
		rec.TenantID = policyDefaultTenantUUID
		rec.PolicyID = p.ID
		rec.Name = p.Name
		rec.DomainUUID = domainUUID
		rec.Rules = rulesJSON
		rec.UpdatedAt = now
		if isNew {
			rec.CreatedAt = now
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.Policy, error) {
	if r.db == nil {
		return nil, model.ErrPolicyNotFound
	}
	var rec PolicyRecord
	if err := r.db.Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrPolicyNotFound
		}
		return nil, err
	}
	var rules model.Rules
	if err := json.Unmarshal(rec.Rules, &rules); err != nil {
		return nil, err
	}
	var bound string
	if rec.DomainUUID != nil {
		if domainID, ok := r.lookupDomainID(*rec.DomainUUID); ok {
			bound = domainID
		}
	}
	return &model.Policy{
		ID:          rec.PolicyID,
		Name:        rec.Name,
		Budget:      rules.Budget,
		Permissions: rules.Permissions,
		Guardrails:  rules.Guardrails,
		BoundDomain: bound,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}, nil
}

// List implements Repository.
func (r *PostgresRepository) List() ([]model.ListItem, error) {
	out := []model.ListItem{}
	if r.db == nil {
		return out, nil
	}
	var rows []PolicyRecord
	if err := r.db.Where("tenant_id = ?", policyDefaultTenantUUID).
		Order("policy_id ASC").
		Find(&rows).Error; err != nil {
		return out, err
	}
	for _, rec := range rows {
		var rules model.Rules
		if err := json.Unmarshal(rec.Rules, &rules); err != nil {
			return out, err
		}
		var bound string
		if rec.DomainUUID != nil {
			if domainID, ok := r.lookupDomainID(*rec.DomainUUID); ok {
				bound = domainID
			}
		}
		out = append(out, model.ListItem{
			ID:          rec.PolicyID,
			Name:        rec.Name,
			Budget:      rules.Budget,
			Permissions: rules.Permissions,
			Guardrails:  rules.Guardrails,
			BoundDomain: bound,
			CreatedAt:   rec.CreatedAt,
			UpdatedAt:   rec.UpdatedAt,
		})
	}
	return out, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil {
		return model.ErrPolicyNotFound
	}
	res := r.db.Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, id).
		Delete(&PolicyRecord{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return model.ErrPolicyNotFound
	}
	return nil
}

// BindDomain implements Repository. Enforces 1:1 binding: if the target
// domain is already bound to a different policy, that policy's binding
// is cleared first so the unique-per-domain invariant holds.
func (r *PostgresRepository) BindDomain(id, domainID string) error {
	if r.db == nil {
		return model.ErrPolicyNotFound
	}
	var target PolicyRecord
	if err := r.db.Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, id).
		Take(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ErrPolicyNotFound
		}
		return err
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if domainID == "" {
			if target.DomainUUID != nil {
				if err := tx.Model(&PolicyRecord{}).
					Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, id).
					Updates(map[string]any{
						"domain_uuid": nil,
						"updated_at":  now,
					}).Error; err != nil {
					return err
				}
			}
			return nil
		}
		uuid, err := r.lookupDomainUUID(domainID)
		if err != nil {
			return err
		}
		// Drop any existing binding for that domain.
		if err := tx.Model(&PolicyRecord{}).
			Where("tenant_id = ? AND domain_uuid = ? AND policy_id <> ?", policyDefaultTenantUUID, uuid, id).
			Updates(map[string]any{
				"domain_uuid": nil,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&PolicyRecord{}).
			Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, id).
			Updates(map[string]any{
				"domain_uuid": uuid,
				"updated_at":  now,
			}).Error
	})
}

// ByDomain implements Repository — used by pkg/policy.Engine to fetch
// the active policy at session start.
func (r *PostgresRepository) ByDomain(domainID string) (*model.Policy, error) {
	if r.db == nil {
		return nil, model.ErrPolicyNotFound
	}
	uuid, err := r.lookupDomainUUID(domainID)
	if err != nil {
		return nil, model.ErrPolicyNotFound
	}
	var rec PolicyRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", policyDefaultTenantUUID, uuid).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrPolicyNotFound
		}
		return nil, err
	}
	var rules model.Rules
	if err := json.Unmarshal(rec.Rules, &rules); err != nil {
		return nil, err
	}
	return &model.Policy{
		ID:          rec.PolicyID,
		Name:        rec.Name,
		Budget:      rules.Budget,
		Permissions: rules.Permissions,
		Guardrails:  rules.Guardrails,
		BoundDomain: domainID,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}, nil
}

func (r *PostgresRepository) lookupDomainUUID(domainID string) (string, error) {
	var rec repository.DomainRecord
	err := r.db.Where("tenant_id = ? AND domain_id = ?", policyDefaultTenantUUID, domainID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", model.ErrPolicyNotFound
		}
		return "", err
	}
	return rec.ID, nil
}

func (r *PostgresRepository) lookupDomainID(uuid string) (string, bool) {
	var rec repository.DomainRecord
	if err := r.db.Where("id = ?", uuid).Take(&rec).Error; err != nil {
		return "", false
	}
	return rec.DomainID, true
}

var _ Repository = (*PostgresRepository)(nil)
