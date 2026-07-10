package repository

import (
	"time"

	"gorm.io/datatypes"
)

// EvalSetRecord is the GORM entity for the `eval_sets` table.
type EvalSetRecord struct {
	ID          string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null"`
	DomainUUID  string         `gorm:"column:domain_uuid;type:uuid;not null;index:idx_eval_sets_domain_uuid"`
	SetID       string         `gorm:"column:set_id;type:varchar(255);not null"`
	Description string         `gorm:"column:description;type:text"`
	Cases       datatypes.JSON `gorm:"column:cases;type:jsonb;not null"`
	CreatedBy   *string        `gorm:"column:created_by;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName returns the table name for GORM.
func (EvalSetRecord) TableName() string { return "eval_sets" }

// EvalCaseRecord is the GORM entity for the `eval_cases` table.
type EvalCaseRecord struct {
	ID          string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null"`
	EvalSetUUID string         `gorm:"column:eval_set_uuid;type:uuid;not null;index:idx_eval_cases_eval_set_uuid"`
	CaseID      string         `gorm:"column:case_id;type:varchar(255);not null"`
	Name        string         `gorm:"column:name;type:varchar(255)"`
	Input       datatypes.JSON `gorm:"column:input;type:jsonb;not null"`
	Expect      datatypes.JSON `gorm:"column:expect;type:jsonb;not null"`
	Scorer      datatypes.JSON `gorm:"column:scorer;type:jsonb"`
	CreatedAt   time.Time
}

// TableName returns the table name for GORM.
func (EvalCaseRecord) TableName() string { return "eval_cases" }

// EvalRunRecord is the GORM entity for the `eval_runs` table.
type EvalRunRecord struct {
	ID              string     `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID        string     `gorm:"column:tenant_id;type:uuid;not null"`
	EvalSetUUID     string     `gorm:"column:eval_set_uuid;type:uuid;not null;index:idx_eval_runs_eval_set_uuid"`
	DomainUUID      string     `gorm:"column:domain_uuid;type:uuid;not null;index:idx_eval_runs_domain_uuid"`
	DomainVersion   string     `gorm:"column:domain_version;type:varchar(64);not null"`
	Orchestration   string     `gorm:"column:orchestration;type:varchar(64);not null"`
	State           string     `gorm:"column:state;type:varchar(64);not null;default:'running'"`
	Score           float64    `gorm:"column:score;type:decimal(5,4)"`
	TotalCases      int        `gorm:"column:total_cases;type:int"`
	PassedCases     int        `gorm:"column:passed_cases;type:int"`
	TotalCostUSD    float64    `gorm:"column:total_cost_usd;type:decimal(12,6)"`
	DurationMs      int64      `gorm:"column:duration_ms;type:bigint"`
	BaselineRunUUID *string    `gorm:"column:baseline_run_uuid;type:uuid"`
	ReportURL       string     `gorm:"column:report_url;type:text"`
	CreatedBy       *string    `gorm:"column:created_by;type:uuid"`
	CreatedAt       time.Time
	CompletedAt     *time.Time `gorm:"column:completed_at;type:timestamptz"`
}

// TableName returns the table name for GORM.
func (EvalRunRecord) TableName() string { return "eval_runs" }

// EvalResultRecord is the GORM entity for the `eval_results` table.
type EvalResultRecord struct {
	ID          string         `gorm:"column:id;type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null"`
	EvalRunUUID string         `gorm:"column:eval_run_uuid;type:uuid;not null;index:idx_eval_results_eval_run_uuid"`
	CaseUUID    string         `gorm:"column:case_uuid;type:uuid;not null;index:idx_eval_results_case_uuid"`
	SessionUUID *string        `gorm:"column:session_uuid;type:uuid"`
	Score       float64        `gorm:"column:score;type:decimal(5,4)"`
	Passed      bool           `gorm:"column:passed;type:boolean;not null;default:false"`
	Actual      datatypes.JSON `gorm:"column:actual;type:jsonb"`
	Details     datatypes.JSON `gorm:"column:details;type:jsonb"`
	DurationMs  int64          `gorm:"column:duration_ms;type:bigint"`
	CostUSD     float64        `gorm:"column:cost_usd;type:decimal(12,6)"`
	CreatedAt   time.Time
}

// TableName returns the table name for GORM.
func (EvalResultRecord) TableName() string { return "eval_results" }
