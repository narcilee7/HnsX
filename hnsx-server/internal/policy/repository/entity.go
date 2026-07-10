package repository

import (
	"time"

	"gorm.io/datatypes"
)

// PolicyRecord is the GORM entity for the `policies` table.
//
// DomainUUID is *string so an unbound policy carries SQL NULL instead of
// the empty-string sentinel — GORM treats "domain_uuid = ''" as a
// distinct value from NULL and that divergence broke earlier 1:1
// binding checks.
type PolicyRecord struct {
	ID         string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID   string         `gorm:"column:tenant_id;type:uuid;not null;index:idx_policies_tenant_id"`
	PolicyID   string         `gorm:"column:policy_id;type:varchar(255);not null;uniqueIndex:idx_policies_tenant_policy"`
	DomainUUID *string        `gorm:"column:domain_uuid;type:uuid;index:idx_policies_domain_uuid"`
	Name       string         `gorm:"column:name;type:varchar(255);not null"`
	Rules      datatypes.JSON `gorm:"column:rules;type:jsonb;not null"`
	CreatedBy  *string        `gorm:"column:created_by;type:uuid"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName returns the table name for GORM.
func (PolicyRecord) TableName() string { return "policies" }
