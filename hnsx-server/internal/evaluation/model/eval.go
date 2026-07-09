// Package model defines the Evaluation aggregate for the HnsX control plane.
package model

import (
	"errors"
	"time"
)

// EvalSet is a collection of test cases for a domain.
type EvalSet struct {
	ID          string
	DomainID    string
	SetID       string
	Description string
	Cases       []EvalCase
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EvalCase is one test case inside an EvalSet.
type EvalCase struct {
	ID     string
	Name   string
	Input  map[string]any
	Expect map[string]any
	Scorer Scorer
}

// Scorer defines how to compare actual output against expectation.
type Scorer struct {
	Type   string
	Config map[string]any
}

// EvalRun is a single execution of an EvalSet against a domain version.
type EvalRun struct {
	ID            string
	EvalSetID     string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         string
	Score         float64
	TotalCases    int
	PassedCases   int
	TotalCostUSD  float64
	DurationMs    int64
	BaselineRunID string
	Results       []EvalResult
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

// EvalResult is the outcome of one EvalCase within a run.
type EvalResult struct {
	ID           string
	CaseID       string
	SessionID    string
	Score        float64
	Passed       bool
	Actual       map[string]any
	Details      map[string]any
	DurationMs   int64
	CostUSD      float64
	CreatedAt    time.Time
}

// Common evaluation errors.
var (
	ErrEvalSetNotFound = errors.New("evaluation: set not found")
	ErrEvalRunNotFound = errors.New("evaluation: run not found")
	ErrEvalCaseExists  = errors.New("evaluation: case already exists")
)
