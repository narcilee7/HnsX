package repository

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
	domainRepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	sessionRepo "github.com/hnsx-io/hnsx/server/internal/session/repository"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const auditDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists audit entries to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed audit repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(e *model.Entry) error {
	if r.db == nil {
		return errors.New("audit/postgres: no database configured")
	}
	if e == nil {
		return model.ErrAuditEntryNotFound
	}
	if err := e.Validate(); err != nil {
		return err
	}

	var domainUUID, sessionUUID *string
	if e.DomainID != "" {
		uuid, err := r.lookupDomainUUID(e.DomainID)
		if err == nil {
			domainUUID = &uuid
		}
	}
	if e.SessionID != "" {
		uuid, err := r.lookupSessionUUID(e.SessionID)
		if err == nil {
			sessionUUID = &uuid
		}
	}

	detailsJSON, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}

	rec := AuditRecord{
		TenantID:     auditDefaultTenantUUID,
		Timestamp:    e.Timestamp,
		SessionUUID:  sessionUUID,
		DomainUUID:   domainUUID,
		Action:       e.Action,
		Actor:        e.Actor,
		ActorType:    e.ActorType,
		Resource:     e.Resource,
		ResourceType: e.ResourceType,
		Decision:     e.Decision,
		Reason:       e.Reason,
		Details:      detailsJSON,
	}
	return r.db.Create(&rec).Error
}

// BySession implements Repository.
func (r *PostgresRepository) BySession(sessionID string) ([]model.Entry, error) {
	if r.db == nil {
		return nil, nil
	}

	sessionUUID, err := r.lookupSessionUUID(sessionID)
	if err != nil {
		return nil, err
	}

	var records []AuditRecord
	if err := r.db.Where("tenant_id = ? AND session_uuid = ?", auditDefaultTenantUUID, sessionUUID).
		Order("timestamp DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return r.toModelList(records)
}

// ByDomain implements Repository.
func (r *PostgresRepository) ByDomain(domainID string) ([]model.Entry, error) {
	if r.db == nil {
		return nil, nil
	}

	domainUUID, err := r.lookupDomainUUID(domainID)
	if err != nil {
		return nil, err
	}

	var records []AuditRecord
	if err := r.db.Where("tenant_id = ? AND domain_uuid = ?", auditDefaultTenantUUID, domainUUID).
		Order("timestamp DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return r.toModelList(records)
}

// List implements Repository.
func (r *PostgresRepository) List(limit, offset int) ([]model.Entry, int, error) {
	if r.db == nil {
		return nil, 0, nil
	}

	var total int64
	if err := r.db.Model(&AuditRecord{}).
		Where("tenant_id = ?", auditDefaultTenantUUID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []AuditRecord
	if err := r.db.Where("tenant_id = ?", auditDefaultTenantUUID).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	entries, err := r.toModelList(records)
	return entries, int(total), err
}

func (r *PostgresRepository) lookupDomainUUID(domainID string) (string, error) {
	var rec domainRepo.DomainRecord
	err := r.db.Where("tenant_id = ? AND domain_id = ?", auditDefaultTenantUUID, domainID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("audit/postgres: domain not found")
		}
		return "", err
	}
	return rec.ID, nil
}

func (r *PostgresRepository) lookupSessionUUID(sessionID string) (string, error) {
	var rec sessionRepo.SessionRecord
	err := r.db.Where("tenant_id = ? AND session_id = ?", auditDefaultTenantUUID, sessionID).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("audit/postgres: session not found")
		}
		return "", err
	}
	return rec.ID, nil
}

func (r *PostgresRepository) toModelList(records []AuditRecord) ([]model.Entry, error) {
	out := make([]model.Entry, 0, len(records))
	for _, rec := range records {
		var details map[string]any
		if len(rec.Details) > 0 {
			if err := json.Unmarshal(rec.Details, &details); err != nil {
				return nil, err
			}
		}
		out = append(out, model.Entry{
			Timestamp:    rec.Timestamp,
			Action:       rec.Action,
			Actor:        rec.Actor,
			ActorType:    rec.ActorType,
			Resource:     rec.Resource,
			ResourceType: rec.ResourceType,
			Decision:     rec.Decision,
			Reason:       rec.Reason,
			Details:      details,
		})
	}
	return out, nil
}

var _ Repository = (*PostgresRepository)(nil)
