package viewmodel

import "time"

// EvalSetListItem is the canonical list view of an eval set.
type EvalSetListItem struct {
	ID          string `json:"id"`
	DomainID    string `json:"domain_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CaseCount   int    `json:"case_count"`
}

// EvalSetList is a paginated list of eval sets.
type EvalSetList struct {
	Items  []EvalSetListItem `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// EvalSetDetail is the canonical detail view of an eval set.
type EvalSetDetail struct {
	ID          string         `json:"id"`
	DomainID    string         `json:"domain_id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Cases       []EvalCaseItem `json:"cases"`
}

// EvalCaseItem is a single eval case.
type EvalCaseItem struct {
	ID          string         `json:"id"`
	Name        string         `json:"name,omitempty"`
	Input       map[string]any `json:"input"`
	Expect      map[string]any `json:"expect,omitempty"`
	Scorer      string         `json:"scorer"`
	Description string         `json:"description,omitempty"`
}

// EvalRunItem is the canonical list view of an eval run.
type EvalRunItem struct {
	ID            string  `json:"id"`
	EvalSetID     string  `json:"set_id"`
	DomainID      string  `json:"domain_id"`
	DomainVersion string  `json:"domain_version"`
	State         string  `json:"state"`
	Score         float64 `json:"score"`
	Total         int     `json:"total"`
	Passed        int     `json:"passed"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	DurationMs    int64   `json:"duration_ms"`
	BaselineRunID string  `json:"baseline_run_id,omitempty"`
}

// EvalRunList is a paginated list of eval runs.
type EvalRunList struct {
	Items  []EvalRunItem `json:"items"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// EvalRunDetail is the canonical detail view of an eval run.
type EvalRunDetail struct {
	ID            string          `json:"id"`
	EvalSetID     string          `json:"set_id"`
	DomainID      string          `json:"domain_id"`
	DomainVersion string          `json:"domain_version"`
	State         string          `json:"state"`
	Score         float64         `json:"score"`
	Total         int             `json:"total"`
	Passed        int             `json:"passed"`
	TotalCostUSD  float64         `json:"total_cost_usd"`
	DurationMs    int64           `json:"duration_ms"`
	BaselineRunID string          `json:"baseline_run_id,omitempty"`
	Cases         []EvalCaseResult `json:"cases"`
}

// EvalCaseResult is a single case result in a run.
type EvalCaseResult struct {
	CaseID    string         `json:"case_id"`
	SessionID string         `json:"session_id,omitempty"`
	Score     float64        `json:"score"`
	Passed    bool           `json:"passed"`
	Actual    map[string]any `json:"actual,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// EvalScorer defines how to score a case.
type EvalScorer struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// EvalCase is one test case inside an EvalSet.
type EvalCase struct {
	ID     string         `json:"id"`
	Name   string         `json:"name,omitempty"`
	Input  map[string]any `json:"input"`
	Expect map[string]any `json:"expect,omitempty"`
	Scorer EvalScorer     `json:"scorer"`
}

// EvalSet is the canonical wire view of an evaluation set.
type EvalSet struct {
	ID          string     `json:"id"`
	SetID       string     `json:"set_id"`
	DomainID    string     `json:"domain_id"`
	Description string     `json:"description,omitempty"`
	Cases       []EvalCase `json:"cases"`
	CaseCount   int        `json:"case_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// EvalRun is the canonical wire view of an evaluation run.
type EvalRun struct {
	ID            string           `json:"id"`
	EvalSetID     string           `json:"eval_set_id"`
	DomainID      string           `json:"domain_id"`
	DomainVersion string           `json:"domain_version"`
	Orchestration string           `json:"orchestration"`
	State         string           `json:"state"`
	Score         float64          `json:"score"`
	TotalCases    int              `json:"total_cases"`
	PassedCases   int              `json:"passed_cases"`
	TotalCostUSD  float64          `json:"total_cost_usd"`
	DurationMs    int64            `json:"duration_ms"`
	BaselineRunID string           `json:"baseline_run_id,omitempty"`
	Cases         []EvalCaseResult `json:"cases"`
	CreatedAt     time.Time        `json:"created_at"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
}

// EvalRunStarted is returned after starting an eval run.
type EvalRunStarted struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
}
