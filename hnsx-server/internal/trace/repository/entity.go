package repository

import (
	"time"

	"gorm.io/datatypes"
)

const traceDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// ObservationRecord is the GORM entity for the `observations` table.
type ObservationRecord struct {
	ID               int64          `gorm:"column:id;primaryKey;autoIncrement"`
	TraceID          string         `gorm:"column:trace_id;type:varchar(64)"`
	SessionID        string         `gorm:"column:session_id;type:varchar(128);not null"`
	DomainID         string         `gorm:"column:domain_id;type:varchar(255)"`
	DomainVersion    string         `gorm:"column:domain_version;type:varchar(64)"`
	StepID           string         `gorm:"column:step_id;type:varchar(255)"`
	AgentID          string         `gorm:"column:agent_id;type:varchar(255)"`
	Kind             string         `gorm:"column:kind;type:varchar(64);not null"`
	Payload          datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CostUSD          float64        `gorm:"column:cost_usd;type:decimal(12,6)"`
	PromptTokens     int            `gorm:"column:prompt_tokens;type:integer"`
	CompletionTokens int            `gorm:"column:completion_tokens;type:integer"`
	LatencyMs        int64          `gorm:"column:latency_ms;type:bigint"`
	CreatedAt        time.Time      `gorm:"column:created_at;type:timestamptz;not null"`
}

// TableName returns the table name for GORM.
func (ObservationRecord) TableName() string { return "observations" }
