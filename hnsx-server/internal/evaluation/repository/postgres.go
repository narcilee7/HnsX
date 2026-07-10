package repository

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const evaluationDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists evaluation sets and runs to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed evaluation repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// SaveSet implements Repository.
func (r *PostgresRepository) SaveSet(set *model.EvalSet) error {
	if r.db == nil {
		return errors.New("evaluation/postgres: no database configured")
	}
	if set == nil || set.ID == "" || set.DomainID == "" {
		return errors.New("evaluation: invalid set")
	}

	domainUUID, err := r.lookupDomainUUID(set.DomainID)
	if err != nil {
		return err
	}

	casesJSON, err := json.Marshal(set.Cases)
	if err != nil {
		return err
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec EvalSetRecord
		err := tx.Where("tenant_id = ? AND set_id = ?", evaluationDefaultTenantUUID, set.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = evaluationDefaultTenantUUID
		rec.DomainUUID = domainUUID
		rec.SetID = set.ID
		rec.Description = set.Description
		rec.Cases = casesJSON

		if isNew {
			rec.CreatedAt = set.CreatedAt
			rec.UpdatedAt = set.UpdatedAt
			if err := tx.Create(&rec).Error; err != nil {
				return err
			}
		} else {
			rec.UpdatedAt = set.UpdatedAt
			if err := tx.Save(&rec).Error; err != nil {
				return err
			}
		}

		for _, c := range set.Cases {
			inputJSON, err := json.Marshal(c.Input)
			if err != nil {
				return err
			}
			expectJSON, err := json.Marshal(c.Expect)
			if err != nil {
				return err
			}
			scorerJSON, err := json.Marshal(c.Scorer)
			if err != nil {
				return err
			}

			var caseRec EvalCaseRecord
			caseErr := tx.Where("tenant_id = ? AND eval_set_uuid = ? AND case_id = ?",
				evaluationDefaultTenantUUID, rec.ID, c.ID).
				Take(&caseRec).Error
			caseIsNew := errors.Is(caseErr, gorm.ErrRecordNotFound)

			caseRec.TenantID = evaluationDefaultTenantUUID
			caseRec.EvalSetUUID = rec.ID
			caseRec.CaseID = c.ID
			caseRec.Name = c.Name
			caseRec.Input = inputJSON
			caseRec.Expect = expectJSON
			caseRec.Scorer = scorerJSON

			if caseIsNew {
				caseRec.CreatedAt = set.CreatedAt
				if err := tx.Create(&caseRec).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Save(&caseRec).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// SetByID implements Repository.
func (r *PostgresRepository) SetByID(id string) (*model.EvalSet, error) {
	if r.db == nil {
		return nil, model.ErrEvalSetNotFound
	}

	var rec EvalSetRecord
	if err := r.db.Where("tenant_id = ? AND set_id = ?", evaluationDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrEvalSetNotFound
		}
		return nil, err
	}

	return r.toEvalSet(rec)
}

// SetsByDomain implements Repository.
func (r *PostgresRepository) SetsByDomain(domainID string) ([]model.EvalSet, error) {
	if r.db == nil {
		return nil, nil
	}

	domainUUID, err := r.lookupDomainUUID(domainID)
	if err != nil {
		return nil, err
	}

	var records []EvalSetRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", evaluationDefaultTenantUUID, domainUUID).
		Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]model.EvalSet, 0, len(records))
	for _, rec := range records {
		set, err := r.toEvalSet(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, *set)
	}
	return out, nil
}

// ListSets implements Repository.
func (r *PostgresRepository) ListSets(limit, offset int) ([]model.EvalSet, int, error) {
	if r.db == nil {
		return nil, 0, nil
	}

	var total int64
	if err := r.db.Model(&EvalSetRecord{}).
		Where("tenant_id = ?", evaluationDefaultTenantUUID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []EvalSetRecord
	if err := r.db.Where("tenant_id = ?", evaluationDefaultTenantUUID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	out := make([]model.EvalSet, 0, len(records))
	for _, rec := range records {
		set, err := r.toEvalSet(rec)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *set)
	}
	return out, int(total), nil
}

// SaveRun implements Repository.
func (r *PostgresRepository) SaveRun(run *model.EvalRun) error {
	if r.db == nil {
		return errors.New("evaluation/postgres: no database configured")
	}
	if run == nil || run.ID == "" || run.EvalSetID == "" || run.DomainID == "" {
		return errors.New("evaluation: invalid run")
	}

	setUUID, err := r.lookupSetUUID(run.EvalSetID)
	if err != nil {
		return err
	}
	domainUUID, err := r.lookupDomainUUID(run.DomainID)
	if err != nil {
		return err
	}

	var baselineUUID *string
	if run.BaselineRunID != "" {
		// BaselineRunID currently stores a run ID string; leave NULL for Phase 2.
	}

	rec := EvalRunRecord{
		ID:              run.ID,
		TenantID:        evaluationDefaultTenantUUID,
		EvalSetUUID:     setUUID,
		DomainUUID:      domainUUID,
		DomainVersion:   run.DomainVersion,
		Orchestration:   run.Orchestration,
		State:           run.State,
		Score:           run.Score,
		TotalCases:      run.TotalCases,
		PassedCases:     run.PassedCases,
		TotalCostUSD:    run.TotalCostUSD,
		DurationMs:      run.DurationMs,
		BaselineRunUUID: baselineUUID,
		CreatedAt:       run.CreatedAt,
		CompletedAt:     run.CompletedAt,
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing EvalRunRecord
		err := tx.Where("id = ?", run.ID).Take(&existing).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		if isNew {
			return tx.Create(&rec).Error
		}

		return tx.Model(&existing).Updates(map[string]any{
			"state":          rec.State,
			"score":          rec.Score,
			"total_cases":    rec.TotalCases,
			"passed_cases":   rec.PassedCases,
			"total_cost_usd": rec.TotalCostUSD,
			"duration_ms":    rec.DurationMs,
			"completed_at":   rec.CompletedAt,
		}).Error
	})
}

// RunByID implements Repository.
func (r *PostgresRepository) RunByID(id string) (*model.EvalRun, error) {
	if r.db == nil {
		return nil, model.ErrEvalRunNotFound
	}

	var rec EvalRunRecord
	if err := r.db.Where("tenant_id = ? AND id = ?", evaluationDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrEvalRunNotFound
		}
		return nil, err
	}

	return r.toEvalRun(rec)
}

// RunsBySet implements Repository.
func (r *PostgresRepository) RunsBySet(setID string) ([]model.EvalRun, error) {
	if r.db == nil {
		return nil, nil
	}

	setUUID, err := r.lookupSetUUID(setID)
	if err != nil {
		return nil, err
	}

	var records []EvalRunRecord
	if err := r.db.Where("tenant_id = ? AND eval_set_uuid = ?", evaluationDefaultTenantUUID, setUUID).
		Order("created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]model.EvalRun, 0, len(records))
	for _, rec := range records {
		run, err := r.toEvalRun(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, nil
}

func (r *PostgresRepository) toEvalSet(rec EvalSetRecord) (*model.EvalSet, error) {
	var domainRec repository.DomainRecord
	if err := r.db.Select("domain_id").
		Where("id = ?", rec.DomainUUID).
		Take(&domainRec).Error; err != nil {
		return nil, err
	}

	var cases []model.EvalCase
	if len(rec.Cases) > 0 {
		if err := json.Unmarshal(rec.Cases, &cases); err != nil {
			return nil, err
		}
	}

	return &model.EvalSet{
		ID:          rec.SetID,
		DomainID:    domainRec.DomainID,
		SetID:       rec.SetID,
		Description: rec.Description,
		Cases:       cases,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}, nil
}

func (r *PostgresRepository) toEvalRun(rec EvalRunRecord) (*model.EvalRun, error) {
	var domainRec repository.DomainRecord
	if err := r.db.Select("domain_id").
		Where("id = ?", rec.DomainUUID).
		Take(&domainRec).Error; err != nil {
		return nil, err
	}

	var setRec EvalSetRecord
	if err := r.db.Select("set_id").
		Where("id = ?", rec.EvalSetUUID).
		Take(&setRec).Error; err != nil {
		return nil, err
	}

	return &model.EvalRun{
		ID:            rec.ID,
		EvalSetID:     setRec.SetID,
		DomainID:      domainRec.DomainID,
		DomainVersion: rec.DomainVersion,
		Orchestration: rec.Orchestration,
		State:         rec.State,
		Score:         rec.Score,
		TotalCases:    rec.TotalCases,
		PassedCases:   rec.PassedCases,
		TotalCostUSD:  rec.TotalCostUSD,
		DurationMs:    rec.DurationMs,
		CreatedAt:     rec.CreatedAt,
		CompletedAt:   rec.CompletedAt,
	}, nil
}

func (r *PostgresRepository) lookupDomainUUID(domainID string) (string, error) {
	var rec repository.DomainRecord
	err := r.db.Where("tenant_id = ? AND domain_id = ?", evaluationDefaultTenantUUID, domainID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("evaluation/postgres: domain not found")
		}
		return "", err
	}
	return rec.ID, nil
}

func (r *PostgresRepository) lookupSetUUID(setID string) (string, error) {
	var rec EvalSetRecord
	err := r.db.Where("tenant_id = ? AND set_id = ?", evaluationDefaultTenantUUID, setID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("evaluation/postgres: set not found")
		}
		return "", err
	}
	return rec.ID, nil
}

var _ Repository = (*PostgresRepository)(nil)
