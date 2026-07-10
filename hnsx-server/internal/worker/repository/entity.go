package repository

import (
	"time"

	"gorm.io/datatypes"
)

// RuntimeRecord is the GORM entity for the `runtimes` table.
type RuntimeRecord struct {
	ID              string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID        string         `gorm:"column:tenant_id;type:uuid;not null;index:idx_runtimes_tenant_id"`
	RuntimeID       string         `gorm:"column:runtime_id;type:varchar(255);not null;uniqueIndex:idx_runtimes_tenant_runtime"`
	Version         string         `gorm:"column:version;type:varchar(64)"`
	Region          string         `gorm:"column:region;type:varchar(128)"`
	Capabilities    datatypes.JSON `gorm:"column:capabilities;type:jsonb"`
	LastHeartbeatAt *time.Time     `gorm:"column:last_heartbeat_at;type:timestamptz"`
	Status          string         `gorm:"column:status;type:varchar(64);not null;default:'active'"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName returns the table name for GORM.
func (RuntimeRecord) TableName() string { return "runtimes" }
