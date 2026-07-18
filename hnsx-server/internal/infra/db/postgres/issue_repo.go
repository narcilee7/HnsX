package postgres

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	"gorm.io/gorm"
)

// ErrIssueNotFound is returned when an issue lookup misses.
var ErrIssueNotFound = errors.New("issue: not found")

// IssueRepo implements issue.Repo against Postgres via GORM.
type IssueRepo struct{ db *DB }

func NewIssueRepo(db *DB) *IssueRepo { return &IssueRepo{db: db} }

func (r *IssueRepo) Create(ctx context.Context, i *issue.Issue) error {
	if err := i.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(i).Error
}

func (r *IssueRepo) Get(ctx context.Context, id string) (*issue.Issue, error) {
	var i issue.Issue
	err := r.db.WithContext(ctx).First(&i, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrIssueNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (r *IssueRepo) ListByWorkspace(ctx context.Context, workspaceID string, f issue.ListFilter) ([]*issue.Issue, error) {
	q := r.db.WithContext(ctx).Model(&issue.Issue{}).Where("workspace_id = ?", workspaceID)
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.AssigneeType != nil {
		q = q.Where("assignee_type = ?", string(*f.AssigneeType))
	}
	if f.AssigneeID != nil {
		q = q.Where("assignee_id = ?", *f.AssigneeID)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var out []*issue.Issue
	if err := q.Order("position ASC, created_at DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *IssueRepo) ListAssignedToAgent(ctx context.Context, agentID string, statuses []issue.Status) ([]*issue.Issue, error) {
	q := r.db.WithContext(ctx).
		Model(&issue.Issue{}).
		Where("assignee_type = ?", string(issue.AssigneeAgent)).
		Where("assignee_id = ?", agentID)
	if len(statuses) > 0 {
		ss := make([]string, len(statuses))
		for i, s := range statuses {
			ss[i] = string(s)
		}
		q = q.Where("status IN ?", ss)
	}
	var out []*issue.Issue
	if err := q.Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *IssueRepo) Update(ctx context.Context, i *issue.Issue) error {
	if err := i.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&issue.Issue{}).
		Where("id = ?", i.ID).
		Updates(map[string]any{
			"title":               i.Title,
			"description":         i.Description,
			"status":              string(i.Status),
			"priority":            string(i.Priority),
			"assignee_type":       i.AssigneeType,
			"assignee_id":         i.AssigneeID,
			"parent_issue_id":     i.ParentIssueID,
			"acceptance_criteria": i.AcceptanceCriteria,
			"context_refs":        i.ContextRefs,
			"position":            i.Position,
			"due_date":            i.DueDate,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIssueNotFound
	}
	return nil
}

func (r *IssueRepo) UpdateStatus(ctx context.Context, id string, status issue.Status) error {
	res := r.db.WithContext(ctx).
		Model(&issue.Issue{}).
		Where("id = ?", id).
		Update("status", string(status))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIssueNotFound
	}
	return nil
}

func (r *IssueRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&issue.Issue{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIssueNotFound
	}
	return nil
}

var _ issue.Repo = (*IssueRepo)(nil)