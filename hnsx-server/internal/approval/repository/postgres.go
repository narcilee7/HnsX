package repository

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/hnsx-io/hnsx/server/internal/approval/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const approvalDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists Approval aggregates to Postgres.
type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// ApprovalRecord is the GORM entity for the `approvals` table.
type ApprovalRecord struct {
	ID          string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null;index:idx_approvals_tenant"`
	ApprovalID  string         `gorm:"column:approval_id;type:varchar(128);not null;uniqueIndex:idx_approvals_tenant_id"`
	SessionID   string         `gorm:"column:session_id;type:varchar(128);not null;index:idx_approvals_session"`
	DomainID    string         `gorm:"column:domain_id;type:varchar(255);index:idx_approvals_domain"`
	Action      string         `gorm:"column:action;type:varchar(255);not null"`
	Resource    string         `gorm:"column:resource;type:varchar(255)"`
	RiskLevel   string         `gorm:"column:risk_level;type:varchar(32)"`
	Context     datatypes.JSON `gorm:"column:context;type:jsonb"`
	Status      string         `gorm:"column:status;type:varchar(32);not null;index:idx_approvals_status"`
	RequestedBy string         `gorm:"column:requested_by;type:varchar(255)"`
	ReviewedBy  *string        `gorm:"column:reviewed_by;type:varchar(255)"`
	Comment     *string        `gorm:"column:comment;type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ResolvedAt  *time.Time `gorm:"column:resolved_at;type:timestamptz"`
}

func (ApprovalRecord) TableName() string { return "approvals" }

func (r *PostgresRepository) Save(a *model.Approval) error {
	if r.db == nil {
		return errors.New("approval/postgres: no database configured")
	}
	ctxJSON, err := json.Marshal(a.Context)
	if err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec ApprovalRecord
		err := tx.Where("tenant_id = ? AND approval_id = ?", approvalDefaultTenantUUID, a.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)
		now := time.Now().UTC()
		rec.TenantID = approvalDefaultTenantUUID
		rec.ApprovalID = a.ID
		rec.SessionID = a.SessionID
		rec.DomainID = a.DomainID
		rec.Action = a.Action
		rec.Resource = a.Resource
		rec.RiskLevel = string(a.RiskLevel)
		rec.Context = ctxJSON
		rec.Status = string(a.Status)
		rec.RequestedBy = a.RequestedBy
		rec.ReviewedBy = nil
		if a.ReviewedBy != "" {
			rec.ReviewedBy = &a.ReviewedBy
		}
		rec.Comment = nil
		if a.Comment != "" {
			rec.Comment = &a.Comment
		}
		rec.UpdatedAt = now
		if isNew {
			rec.CreatedAt = now
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

func (r *PostgresRepository) ByID(id string) (*model.Approval, error) {
	if r.db == nil {
		return nil, model.ErrApprovalNotFound
	}
	var rec ApprovalRecord
	if err := r.db.Where("tenant_id = ? AND approval_id = ?", approvalDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrApprovalNotFound
		}
		return nil, err
	}
	return r.toModel(rec), nil
}

func (r *PostgresRepository) List(filter ListFilter) ([]model.ListItem, error) {
	out := []model.ListItem{}
	if r.db == nil {
		return out, nil
	}
	q := r.db.Model(&ApprovalRecord{}).Where("tenant_id = ?", approvalDefaultTenantUUID)
	if filter.DomainID != "" {
		q = q.Where("domain_id = ?", filter.DomainID)
	}
	if filter.SessionID != "" {
		q = q.Where("session_id = ?", filter.SessionID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var rows []ApprovalRecord
	if err := q.Order("created_at DESC, approval_id DESC").Find(&rows).Error; err != nil {
		return out, err
	}
	for _, rec := range rows {
		out = append(out, model.ListItem{
			ID:          rec.ApprovalID,
			SessionID:   rec.SessionID,
			DomainID:    rec.DomainID,
			Action:      rec.Action,
			Resource:    rec.Resource,
			RiskLevel:   model.RiskLevel(rec.RiskLevel),
			Status:      model.Status(rec.Status),
			RequestedBy: rec.RequestedBy,
			CreatedAt:   rec.CreatedAt,
			UpdatedAt:   rec.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *PostgresRepository) PendingForSession(sessionID string) (*model.Approval, error) {
	if r.db == nil {
		return nil, model.ErrApprovalNotFound
	}
	var rec ApprovalRecord
	if err := r.db.Where("tenant_id = ? AND session_id = ? AND status = ?", approvalDefaultTenantUUID, sessionID, string(model.StatusPending)).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrApprovalNotFound
		}
		return nil, err
	}
	return r.toModel(rec), nil
}

func (r *PostgresRepository) Resolve(id, decidedBy, comment string, status model.Status) error {
	if r.db == nil {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":      string(status),
		"reviewed_by": decidedBy,
		"updated_at":  now,
		"resolved_at": now,
	}
	if comment != "" {
		updates["comment"] = comment
	}
	res := r.db.Model(&ApprovalRecord{}).
		Where("tenant_id = ? AND approval_id = ? AND status = ?", approvalDefaultTenantUUID, id, string(model.StatusPending)).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return model.ErrAlreadyResolved
	}
	return nil
}

func (r *PostgresRepository) toModel(rec ApprovalRecord) *model.Approval {
	ctx := map[string]any{}
	if len(rec.Context) > 0 {
		_ = json.Unmarshal(rec.Context, &ctx)
	}
	a := &model.Approval{
		ID:          rec.ApprovalID,
		SessionID:   rec.SessionID,
		DomainID:    rec.DomainID,
		Action:      rec.Action,
		Resource:    rec.Resource,
		RiskLevel:   model.RiskLevel(rec.RiskLevel),
		Context:     ctx,
		Status:      model.Status(rec.Status),
		RequestedBy: rec.RequestedBy,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}
	if rec.ReviewedBy != nil {
		a.ReviewedBy = *rec.ReviewedBy
	}
	if rec.Comment != nil {
		a.Comment = *rec.Comment
	}
	if rec.ResolvedAt != nil {
		t := *rec.ResolvedAt
		a.ResolvedAt = &t
	}
	return a
}

var _ Repository = (*PostgresRepository)(nil)
