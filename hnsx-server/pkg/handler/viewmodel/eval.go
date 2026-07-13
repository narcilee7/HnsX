package viewmodel

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

// EvalRunStarted is returned after starting an eval run.
type EvalRunStarted struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
}
