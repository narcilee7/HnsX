// Package model defines the Evaluation aggregate for the HnsX control plane.
package model

import (
	"errors"
	"time"
)

// EvalSet is a collection of test cases for a domain.
type EvalSet struct {
	ID          string    `json:"id"`
	DomainID    string    `json:"domain_id"`
	SetID       string    `json:"set_id"`
	Description string    `json:"description"`
	Cases       []EvalCase `json:"cases"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EvalCase is one test case inside an EvalSet.
type EvalCase struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Input  map[string]any `json:"input"`
	Expect map[string]any `json:"expect"`
	Scorer Scorer         `json:"scorer"`
}

// Scorer defines how to compare actual output against expectation.
type Scorer struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// EvalRun is a single execution of an EvalSet against a domain version.
type EvalRun struct {
	ID            string       `json:"id"`
	EvalSetID     string       `json:"eval_set_id"`
	DomainID      string       `json:"domain_id"`
	DomainVersion string       `json:"domain_version"`
	Orchestration string       `json:"orchestration"`
	State         string       `json:"state"`
	Score         float64      `json:"score"`
	TotalCases    int          `json:"total_cases"`
	PassedCases   int          `json:"passed_cases"`
	TotalCostUSD  float64      `json:"total_cost_usd"`
	DurationMs    int64        `json:"duration_ms"`
	BaselineRunID string       `json:"baseline_run_id"`
	Results       []EvalResult `json:"results"`
	CreatedAt     time.Time    `json:"created_at"`
	CompletedAt   *time.Time   `json:"completed_at"`
}

// EvalResult is the outcome of one EvalCase within a run.
type EvalResult struct {
	ID         string         `json:"id"`
	CaseID     string         `json:"case_id"`
	SessionID  string         `json:"session_id"`
	Score      float64        `json:"score"`
	Passed     bool           `json:"passed"`
	Actual     map[string]any `json:"actual"`
	Details    map[string]any `json:"details"`
	DurationMs int64          `json:"duration_ms"`
	CostUSD    float64        `json:"cost_usd"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Common evaluation errors.
var (
	ErrEvalSetNotFound = errors.New("evaluation: set not found")
	ErrEvalRunNotFound = errors.New("evaluation: run not found")
	ErrEvalCaseExists  = errors.New("evaluation: case already exists")
)
