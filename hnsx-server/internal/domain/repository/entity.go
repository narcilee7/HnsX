package repository

import (
	"time"

	"gorm.io/datatypes"
)

// DomainRecord is the GORM entity for the `domains` table.
type DomainRecord struct {
	ID             string `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID       string `gorm:"column:tenant_id;type:uuid;not null;index:idx_domains_tenant_id"`
	DomainID       string `gorm:"column:domain_id;type:varchar(255);not null;index:idx_domains_domain_id"`
	CurrentVersion string `gorm:"column:current_version;type:varchar(64);not null"`
	Description    string `gorm:"column:description;type:text"`
	Status         string `gorm:"column:status;type:varchar(64);not null;default:'active'"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the table name for GORM.
func (DomainRecord) TableName() string { return "domains" }

// DomainVersionRecord is the GORM entity for the `domain_versions` table.
type DomainVersionRecord struct {
	ID           string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID     string         `gorm:"column:tenant_id;type:uuid;not null"`
	DomainUUID   string         `gorm:"column:domain_uuid;type:uuid;not null;index:idx_domain_versions_domain_uuid"`
	Version      string         `gorm:"column:version;type:varchar(64);not null"`
	YAMLBody     string         `gorm:"column:yaml_body;type:text;not null"`
	JSONBody     datatypes.JSON `gorm:"column:json_body;type:jsonb"`
	HarnessHash  string         `gorm:"column:harness_hash;type:varchar(64);not null"`
	CreatedBy    *string        `gorm:"column:created_by;type:uuid"`
	CreatedAt    time.Time
}

// TableName returns the table name for GORM.
func (DomainVersionRecord) TableName() string { return "domain_versions" }
