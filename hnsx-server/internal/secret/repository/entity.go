package repository

import (
	"time"
)

// SecretRecord is the GORM entity for the `secrets` table.
type SecretRecord struct {
	ID          string     `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    string     `gorm:"column:tenant_id;type:uuid;not null;index:idx_secrets_tenant_id"`
	SecretID    string     `gorm:"column:secret_id;type:varchar(255);not null;uniqueIndex:idx_secrets_tenant_secret"`
	Value       string     `gorm:"column:value;type:text;not null"`
	Description string     `gorm:"column:description;type:text"`
	Kind        string     `gorm:"column:kind;type:varchar(64);default:'generic'"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at;type:timestamptz"`
	CreatedBy   *string    `gorm:"column:created_by;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName returns the table name for GORM.
func (SecretRecord) TableName() string { return "secrets" }
