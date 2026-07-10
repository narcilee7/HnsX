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

// Save implements Repository.
func (r *PostgresRepository) Save(p *model.Policy) error {
	if r.db == nil {
		return errors.New("policy/postgres: no database configured")
	}
	if p == nil || p.DomainID == "" {
		return errors.New("policy: invalid policy")
	}

	domainUUID, err := r.lookupDomainUUID(p.DomainID)
	if err != nil {
		return err
	}

	rules := map[string]any{
		"budget":      p.Budget,
		"permissions": p.Permissions,
		"guardrails":  p.Guardrails,
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec PolicyRecord
		err := tx.Where("tenant_id = ? AND policy_id = ?", policyDefaultTenantUUID, p.DomainID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = policyDefaultTenantUUID
		rec.PolicyID = p.DomainID
		rec.DomainUUID = domainUUID
		rec.Name = p.DomainID
		rec.Rules = rulesJSON
		rec.UpdatedAt = now
		if isNew {
			rec.CreatedAt = now
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) (*model.Policy, error) {
	if r.db == nil {
		return nil, model.ErrPolicyNotFound
	}

	domainUUID, err := r.lookupDomainUUID(domainID)
	if err != nil {
		return nil, model.ErrPolicyNotFound
	}

	var rec PolicyRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", policyDefaultTenantUUID, domainUUID).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrPolicyNotFound
		}
		return nil, err
	}

	var rules struct {
		Budget      model.Budget      `json:"budget"`
		Permissions model.Permissions `json:"permissions"`
		Guardrails  []model.Guardrail `json:"guardrails"`
	}
	if err := json.Unmarshal(rec.Rules, &rules); err != nil {
		return nil, err
	}

	return &model.Policy{
		DomainID:    domainID,
		Budget:      rules.Budget,
		Permissions: rules.Permissions,
		Guardrails:  rules.Guardrails,
	}, nil
}

func (r *PostgresRepository) lookupDomainUUID(domainID string) (string, error) {
	var rec repository.DomainRecord
	err := r.db.Where("tenant_id = ? AND domain_id = ?", policyDefaultTenantUUID, domainID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("policy/postgres: domain not found")
		}
		return "", err
	}
	return rec.ID, nil
}

var _ Repository = (*PostgresRepository)(nil)
