package repository

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

const sessionDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists Session aggregates to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed session repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(s *model.Session) error {
	if r.db == nil {
		return errors.New("session/postgres: no database configured")
	}
	if s == nil || s.ID == "" {
		return model.ErrInvalidSession
	}

	domainUUID, err := r.lookupDomainUUID(s.DomainID)
	if err != nil {
		return err
	}

	triggerJSON, err := json.Marshal(s.Trigger)
	if err != nil {
		return err
	}
	var resultJSON []byte
	if s.Result != nil {
		resultJSON, err = json.Marshal(s.Result)
		if err != nil {
			return err
		}
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec SessionRecord
		err := tx.Where("tenant_id = ? AND session_id = ?", sessionDefaultTenantUUID, s.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = sessionDefaultTenantUUID
		rec.SessionID = s.ID
		rec.DomainUUID = domainUUID
		rec.DomainVersion = s.DomainVersion
		rec.Orchestration = s.Orchestration
		rec.State = string(s.State)
		rec.TriggerPayload = triggerJSON
		rec.ResultPayload = resultJSON
		rec.StartedAt = &s.StartedAt
		rec.CompletedAt = s.CompletedAt

		if isNew {
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.Session, error) {
	if r.db == nil {
		return nil, model.ErrSessionNotFound
	}

	var rec SessionRecord
	if err := r.db.Where("tenant_id = ? AND session_id = ?", sessionDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrSessionNotFound
		}
		return nil, err
	}

	return r.toModel(rec)
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.Session, error) {
	if r.db == nil {
		return nil, nil
	}

	var records []SessionRecord
	if err := r.db.Where("tenant_id = ?", sessionDefaultTenantUUID).Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]*model.Session, 0, len(records))
	for _, rec := range records {
		s, err := r.toModel(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) ([]*model.Session, error) {
	if r.db == nil {
		return nil, nil
	}

	domainUUID, err := r.lookupDomainUUID(domainID)
	if err != nil {
		return nil, err
	}

	var records []SessionRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", sessionDefaultTenantUUID, domainUUID).
		Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]*model.Session, 0, len(records))
	for _, rec := range records {
		s, err := r.toModel(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil {
		return nil
	}

	return r.db.Where("tenant_id = ? AND session_id = ?", sessionDefaultTenantUUID, id).
		Delete(&SessionRecord{}).Error
}

func (r *PostgresRepository) lookupDomainUUID(domainID string) (string, error) {
	var rec repository.DomainRecord
	err := r.db.Where("tenant_id = ? AND domain_id = ?", sessionDefaultTenantUUID, domainID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", model.ErrInvalidSession
		}
		return "", err
	}
	return rec.ID, nil
}

func (r *PostgresRepository) toModel(rec SessionRecord) (*model.Session, error) {
	var domainRec repository.DomainRecord
	if err := r.db.Select("domain_id").
		Where("id = ?", rec.DomainUUID).
		Take(&domainRec).Error; err != nil {
		return nil, err
	}

	var trigger map[string]any
	if len(rec.TriggerPayload) > 0 {
		if err := json.Unmarshal(rec.TriggerPayload, &trigger); err != nil {
			return nil, err
		}
	}

	var result *runtime.Result
	if len(rec.ResultPayload) > 0 {
		result = &runtime.Result{}
		if err := json.Unmarshal(rec.ResultPayload, result); err != nil {
			return nil, err
		}
	}

	startedAt := rec.StartedAt
	if startedAt == nil {
		startedAt = &rec.CreatedAt
	}

	return &model.Session{
		ID:            rec.SessionID,
		DomainID:      domainRec.DomainID,
		DomainVersion: rec.DomainVersion,
		Orchestration: rec.Orchestration,
		State:         model.State(rec.State),
		Trigger:       trigger,
		Result:        result,
		StartedAt:     *startedAt,
		CompletedAt:   rec.CompletedAt,
	}, nil
}

var _ Repository = (*PostgresRepository)(nil)
