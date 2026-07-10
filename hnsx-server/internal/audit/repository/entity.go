package repository

import (
	"time"

	"gorm.io/datatypes"
)

// AuditRecord is the GORM entity for the `audit_logs` table.
type AuditRecord struct {
	ID           string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID     string         `gorm:"column:tenant_id;type:uuid;not null;index:idx_audit_logs_tenant_id"`
	Timestamp    time.Time      `gorm:"column:timestamp;type:timestamptz;not null;default:now()"`
	SessionUUID  *string        `gorm:"column:session_uuid;type:uuid;index:idx_audit_logs_session_uuid"`
	DomainUUID   *string        `gorm:"column:domain_uuid;type:uuid"`
	Action       string         `gorm:"column:action;type:varchar(255);not null;index:idx_audit_logs_action"`
	Actor        string         `gorm:"column:actor;type:varchar(255);not null;index:idx_audit_logs_actor"`
	ActorType    string         `gorm:"column:actor_type;type:varchar(64);not null"`
	Resource     string         `gorm:"column:resource;type:varchar(255)"`
	ResourceType string         `gorm:"column:resource_type;type:varchar(64)"`
	Decision     string         `gorm:"column:decision;type:varchar(64)"`
	Reason       string         `gorm:"column:reason;type:text"`
	Details      datatypes.JSON `gorm:"column:details;type:jsonb"`
	CreatedAt    time.Time
}

// TableName returns the table name for GORM.
func (AuditRecord) TableName() string { return "audit_logs" }
