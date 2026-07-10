package repository

import (
	"time"

	"gorm.io/datatypes"
)

// SessionRecord is the GORM entity for the `sessions` table.
type SessionRecord struct {
	ID             string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID       string         `gorm:"column:tenant_id;type:uuid;not null;index:idx_sessions_tenant_id"`
	SessionID      string         `gorm:"column:session_id;type:varchar(255);not null;uniqueIndex:idx_sessions_tenant_session"`
	DomainUUID     string         `gorm:"column:domain_uuid;type:uuid;not null;index:idx_sessions_domain_uuid"`
	DomainVersion  string         `gorm:"column:domain_version;type:varchar(64);not null"`
	Orchestration  string         `gorm:"column:orchestration;type:varchar(64);not null"`
	State          string         `gorm:"column:state;type:varchar(64);not null;default:'pending';index:idx_sessions_state"`
	TriggerPayload datatypes.JSON `gorm:"column:trigger_payload;type:jsonb"`
	ResultPayload  datatypes.JSON `gorm:"column:result_payload;type:jsonb"`
	TraceID        string         `gorm:"column:trace_id;type:varchar(255)"`
	RuntimeID      string         `gorm:"column:runtime_id;type:varchar(255)"`
	StartedAt      *time.Time     `gorm:"column:started_at;type:timestamptz"`
	CompletedAt    *time.Time     `gorm:"column:completed_at;type:timestamptz"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the table name for GORM.
func (SessionRecord) TableName() string { return "sessions" }
