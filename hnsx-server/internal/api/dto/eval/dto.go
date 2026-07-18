package eval

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
)

type Case struct {
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Expected json.RawMessage `json:"expected,omitempty"`
	Scorer   string          `json:"scorer"`
	Weight   float64         `json:"weight"`
}

type CreateRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Cases       []Case `json:"cases"`
	Version     string `json:"version"`
}

type Response struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Cases       json.RawMessage `json:"cases"`
	Version     string          `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type RunResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	EvalSetID   string          `json:"eval_set_id"`
	IssueID     *string         `json:"issue_id,omitempty"`
	HarnessID   *string         `json:"harness_id,omitempty"`
	Status      string          `json:"status"`
	TotalScore  float64         `json:"total_score"`
	Results     json.RawMessage `json:"results"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       string          `json:"error"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

type ListRunsResponse struct {
	Items []RunResponse `json:"items"`
	Total int           `json:"total"`
}

func FromDomainSet(e *eval.EvalSet) Response {
	return Response{
		ID:          e.ID,
		WorkspaceID: e.WorkspaceID,
		Name:        e.Name,
		Description: e.Description,
		Cases:       e.Cases,
		Version:     e.Version,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func FromDomainSetList(items []*eval.EvalSet) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, e := range items {
		out.Items = append(out.Items, FromDomainSet(e))
	}
	out.Total = len(out.Items)
	return out
}

func FromDomainRun(r *eval.Run) RunResponse {
	return RunResponse{
		ID:          r.ID,
		WorkspaceID: r.WorkspaceID,
		EvalSetID:   r.EvalSetID,
		IssueID:     r.IssueID,
		HarnessID:   r.HarnessID,
		Status:      string(r.Status),
		TotalScore:  r.TotalScore,
		Results:     r.Results,
		StartedAt:   r.StartedAt,
		CompletedAt: r.CompletedAt,
		Error:       r.Error,
	}
}

func FromDomainRunList(items []*eval.Run) ListRunsResponse {
	out := ListRunsResponse{Items: make([]RunResponse, 0, len(items))}
	for _, r := range items {
		out.Items = append(out.Items, FromDomainRun(r))
	}
	out.Total = len(out.Items)
	return out
}