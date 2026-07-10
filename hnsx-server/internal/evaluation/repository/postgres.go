package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const evalDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists evaluation sets and runs to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed evaluation repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// SaveSet implements Repository.
func (r *PostgresRepository) SaveSet(set *model.EvalSet) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("evaluation/postgres: no database configured")
	}
	if set == nil || set.ID == "" || set.DomainID == "" {
		return errors.New("evaluation: invalid set")
	}

	ctx := context.Background()
	domainUUID, err := r.lookupDomainUUID(ctx, set.DomainID)
	if err != nil {
		return err
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	casesJSON, err := json.Marshal(set.Cases)
	if err != nil {
		return err
	}

	var setUUID string
	err = tx.QueryRow(ctx, `
		INSERT INTO eval_sets (tenant_id, domain_uuid, set_id, description, cases, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6)
		ON CONFLICT (tenant_id, domain_uuid, set_id) DO UPDATE
		SET description = EXCLUDED.description,
		    cases = EXCLUDED.cases,
		    updated_at = EXCLUDED.updated_at
		RETURNING id
	`, evalDefaultTenantUUID, domainUUID, set.ID, set.Description, string(casesJSON), set.UpdatedAt).Scan(&setUUID)
	if err != nil {
		return err
	}

	for _, c := range set.Cases {
		inputJSON, _ := json.Marshal(c.Input)
		expectJSON, _ := json.Marshal(c.Expect)
		scorerJSON, _ := json.Marshal(c.Scorer)
		_, err = tx.Exec(ctx, `
			INSERT INTO eval_cases (tenant_id, eval_set_uuid, case_id, name, input, expect, scorer, created_at)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, NOW())
			ON CONFLICT (tenant_id, eval_set_uuid, case_id) DO UPDATE
			SET name = EXCLUDED.name,
			    input = EXCLUDED.input,
			    expect = EXCLUDED.expect,
			    scorer = EXCLUDED.scorer
		`, evalDefaultTenantUUID, setUUID, c.ID, c.Name, string(inputJSON), string(expectJSON), string(scorerJSON))
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// SetByID implements Repository.
func (r *PostgresRepository) SetByID(id string) (*model.EvalSet, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrEvalSetNotFound
	}

	ctx := context.Background()
	row := r.db.Pool.QueryRow(ctx, `
		SELECT es.id, d.domain_id, es.set_id, es.description, es.cases, es.created_at, es.updated_at
		FROM eval_sets es
		JOIN domains d ON d.id = es.domain_uuid
		WHERE es.tenant_id = $1::uuid AND es.set_id = $2
		LIMIT 1
	`, evalDefaultTenantUUID, id)

	return r.scanSet(row)
}

// SetsByDomain implements Repository.
func (r *PostgresRepository) SetsByDomain(domainID string) ([]model.EvalSet, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT es.id, d.domain_id, es.set_id, es.description, es.cases, es.created_at, es.updated_at
		FROM eval_sets es
		JOIN domains d ON d.id = es.domain_uuid
		WHERE es.tenant_id = $1::uuid AND d.domain_id = $2
	`, evalDefaultTenantUUID, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.EvalSet
	for rows.Next() {
		set, err := r.scanSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *set)
	}
	return out, rows.Err()
}

// ListSets implements Repository.
func (r *PostgresRepository) ListSets(limit, offset int) ([]model.EvalSet, int, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, 0, nil
	}

	ctx := context.Background()
	var total int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM eval_sets WHERE tenant_id = $1::uuid
	`, evalDefaultTenantUUID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT es.id, d.domain_id, es.set_id, es.description, es.cases, es.created_at, es.updated_at
		FROM eval_sets es
		JOIN domains d ON d.id = es.domain_uuid
		WHERE es.tenant_id = $1::uuid
		ORDER BY es.created_at DESC
		LIMIT $2 OFFSET $3
	`, evalDefaultTenantUUID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []model.EvalSet
	for rows.Next() {
		set, err := r.scanSet(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *set)
	}
	return out, total, rows.Err()
}

// SaveRun implements Repository.
func (r *PostgresRepository) SaveRun(run *model.EvalRun) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("evaluation/postgres: no database configured")
	}
	if run == nil || run.ID == "" || run.EvalSetID == "" || run.DomainID == "" {
		return errors.New("evaluation: invalid run")
	}

	ctx := context.Background()
	setUUID, err := r.lookupSetUUID(ctx, run.EvalSetID)
	if err != nil {
		return err
	}
	domainUUID, err := r.lookupDomainUUID(ctx, run.DomainID)
	if err != nil {
		return err
	}

	var baselineUUID *string
	if run.BaselineRunID != "" {
		// BaselineRunID currently stores a run ID string; look it up if needed.
		// For Phase 2 we leave it NULL because the model stores string IDs.
	}

	var completedAt *time.Time
	if run.CompletedAt != nil {
		completedAt = run.CompletedAt
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO eval_runs (
			tenant_id, eval_set_uuid, domain_uuid, domain_version, orchestration, state,
			score, total_cases, passed_cases, total_cost_usd, duration_ms,
			baseline_run_uuid, created_at, completed_at
		)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid, $13, $14)
		ON CONFLICT (id) DO UPDATE
		SET state = EXCLUDED.state,
		    score = EXCLUDED.score,
		    total_cases = EXCLUDED.total_cases,
		    passed_cases = EXCLUDED.passed_cases,
		    total_cost_usd = EXCLUDED.total_cost_usd,
		    duration_ms = EXCLUDED.duration_ms,
		    completed_at = EXCLUDED.completed_at
	`, evalDefaultTenantUUID, setUUID, domainUUID, run.DomainVersion, run.Orchestration, run.State,
		run.Score, run.TotalCases, run.PassedCases, run.TotalCostUSD, run.DurationMs,
		baselineUUID, run.CreatedAt, completedAt)
	return err
}

// RunByID implements Repository.
func (r *PostgresRepository) RunByID(id string) (*model.EvalRun, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrEvalRunNotFound
	}

	ctx := context.Background()
	row := r.db.Pool.QueryRow(ctx, `
		SELECT er.id, d.domain_id, er.eval_set_uuid, es.set_id, er.domain_version, er.orchestration,
		       er.state, er.score, er.total_cases, er.passed_cases, er.total_cost_usd,
		       er.duration_ms, er.created_at, er.completed_at
		FROM eval_runs er
		JOIN domains d ON d.id = er.domain_uuid
		JOIN eval_sets es ON es.id = er.eval_set_uuid
		WHERE er.tenant_id = $1::uuid AND er.id = $2
		LIMIT 1
	`, evalDefaultTenantUUID, id)

	return r.scanRun(row)
}

// RunsBySet implements Repository.
func (r *PostgresRepository) RunsBySet(setID string) ([]model.EvalRun, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT er.id, d.domain_id, er.eval_set_uuid, es.set_id, er.domain_version, er.orchestration,
		       er.state, er.score, er.total_cases, er.passed_cases, er.total_cost_usd,
		       er.duration_ms, er.created_at, er.completed_at
		FROM eval_runs er
		JOIN domains d ON d.id = er.domain_uuid
		JOIN eval_sets es ON es.id = er.eval_set_uuid
		WHERE er.tenant_id = $1::uuid AND es.set_id = $2
		ORDER BY er.created_at DESC
	`, evalDefaultTenantUUID, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.EvalRun
	for rows.Next() {
		run, err := r.scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) scanSet(row interface {
	Scan(dest ...any) error
}) (*model.EvalSet, error) {
	var set model.EvalSet
	var setUUID, domainUUID string
	var casesJSON []byte
	err := row.Scan(&setUUID, &domainUUID, &set.ID, &set.Description, &casesJSON, &set.CreatedAt, &set.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrEvalSetNotFound
		}
		return nil, err
	}
	set.DomainID = domainUUID
	set.SetID = set.ID
	if len(casesJSON) > 0 {
		_ = json.Unmarshal(casesJSON, &set.Cases)
	}
	return &set, nil
}

func (r *PostgresRepository) scanRun(row interface {
	Scan(dest ...any) error
}) (*model.EvalRun, error) {
	var run model.EvalRun
	var runUUID, domainUUID, setUUID, setID string
	var completedAt *time.Time
	err := row.Scan(
		&runUUID, &domainUUID, &setUUID, &setID, &run.DomainVersion, &run.Orchestration,
		&run.State, &run.Score, &run.TotalCases, &run.PassedCases, &run.TotalCostUSD,
		&run.DurationMs, &run.CreatedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrEvalRunNotFound
		}
		return nil, err
	}
	run.ID = runUUID
	run.DomainID = domainUUID
	run.EvalSetID = setID
	run.CompletedAt = completedAt
	return &run, nil
}

func (r *PostgresRepository) lookupDomainUUID(ctx context.Context, domainID string) (string, error) {
	var domainUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM domains
		WHERE tenant_id = $1::uuid AND domain_id = $2
		LIMIT 1
	`, evalDefaultTenantUUID, domainID).Scan(&domainUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("evaluation/postgres: domain not found")
		}
		return "", err
	}
	return domainUUID, nil
}

func (r *PostgresRepository) lookupSetUUID(ctx context.Context, setID string) (string, error) {
	var setUUID string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id FROM eval_sets
		WHERE tenant_id = $1::uuid AND set_id = $2
		LIMIT 1
	`, evalDefaultTenantUUID, setID).Scan(&setUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("evaluation/postgres: set not found")
		}
		return "", err
	}
	return setUUID, nil
}

var _ Repository = (*PostgresRepository)(nil)
