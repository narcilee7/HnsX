package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/policy/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const policyDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists policy aggregates to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed policy repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(p *model.Policy) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("policy/postgres: no database configured")
	}
	if p == nil || p.DomainID == "" {
		return errors.New("policy: invalid policy")
	}

	ctx := context.Background()
	domainUUID, err := r.lookupDomainUUID(ctx, p.DomainID)
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

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO policies (tenant_id, policy_id, domain_uuid, name, rules, updated_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5::jsonb, NOW())
		ON CONFLICT (tenant_id, policy_id) DO UPDATE
		SET domain_uuid = EXCLUDED.domain_uuid,
		    name = EXCLUDED.name,
		    rules = EXCLUDED.rules,
		    updated_at = NOW()
	`, policyDefaultTenantUUID, p.DomainID, domainUUID, p.DomainID, string(rulesJSON))
	return err
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) (*model.Policy, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrPolicyNotFound
	}

	ctx := context.Background()
	domainUUID, err := r.lookupDomainUUID(ctx, domainID)
	if err != nil {
		return nil, model.ErrPolicyNotFound
	}

	var rulesJSON []byte
	err = r.db.Pool.QueryRow(ctx, `
		SELECT rules
		FROM policies
		WHERE tenant_id = $1::uuid AND domain_uuid = $2::uuid
		LIMIT 1
	`, policyDefaultTenantUUID, domainUUID).Scan(&rulesJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrPolicyNotFound
		}
		return nil, err
	}

	var rules struct {
		Budget      model.Budget      `json:"budget"`
		Permissions model.Permissions `json:"permissions"`
		Guardrails  []model.Guardrail `json:"guardrails"`
	}
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, err
	}

	return &model.Policy{
		DomainID:    domainID,
		Budget:      rules.Budget,
		Permissions: rules.Permissions,
		Guardrails:  rules.Guardrails,
	}, nil
}

func (r *PostgresRepository) lookupDomainUUID(ctx context.Context, domainID string) (string, error) {
	var domainUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
		LIMIT 1
	`, policyDefaultTenantUUID, domainID).Scan(&domainUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("policy/postgres: domain not found")
		}
		return "", err
	}
	return domainUUID, nil
}

var _ Repository = (*PostgresRepository)(nil)
