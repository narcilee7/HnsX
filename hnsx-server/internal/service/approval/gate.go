// Package approval hosts the application-level Gate implementation
// that bridges the daemon_runtime to human-in-the-loop approval.
//
// The Gate is the runtime hook: when the policy engine flags an action
// as approval_required, daemon_runtime calls Gate.Request + Gate.Wait
// which create a pending Approval row, block the calling goroutine
// until the human (via CLI or webhook) grants or denies, and return
// the resulting Status to the daemon so it can resume or abort the
// agent session.
package approval

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
)

// Gate implements approval.Gate against the approval Repo.
type Gate struct {
	repo        approval.Repo
	logger      *slog.Logger
	pollEvery   time.Duration
	defaultWait time.Duration
}

// GateConfig configures the Gate.
type GateConfig struct {
	Repo        approval.Repo
	Logger      *slog.Logger
	PollEvery   time.Duration // how often to poll Approval status; default 1s
	DefaultWait time.Duration // how long to wait for a decision; default 24h
}

// NewGate constructs a Gate.
func NewGate(cfg GateConfig) *Gate {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = time.Second
	}
	if cfg.DefaultWait <= 0 {
		cfg.DefaultWait = 24 * time.Hour
	}
	return &Gate{
		repo:        cfg.Repo,
		logger:      cfg.Logger,
		pollEvery:   cfg.PollEvery,
		defaultWait: cfg.DefaultWait,
	}
}

// Request persists a pending approval row and returns its ID. The
// caller is expected to follow up with Wait to block on the decision.
func (g *Gate) Request(ctx context.Context, a *approval.Approval) error {
	a.Status = approval.StatusPending
	if err := g.repo.Create(ctx, a); err != nil {
		return err
	}
	g.logger.Info("approval: request created",
		"id", a.ID,
		"workspace", a.WorkspaceID,
		"issue", a.IssueID,
		"action", a.Action,
	)
	return nil
}

// Wait polls the approval until it transitions to granted / denied /
// expired. Returns the final status (or ctx.Err() on cancellation /
// timeout). It does NOT itself change the status — Grant/Deny is a
// separate human action.
func (g *Gate) Wait(ctx context.Context, approvalID string) (approval.Status, error) {
	deadline := time.Now().Add(g.defaultWait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	t := time.NewTicker(g.pollEvery)
	defer t.Stop()

	for {
		a, err := g.repo.Get(ctx, approvalID)
		if err != nil {
			return "", err
		}
		switch a.Status {
		case approval.StatusGranted, approval.StatusDenied, approval.StatusExpired:
			g.logger.Info("approval: decision reached",
				"id", a.ID, "status", a.Status,
			)
			return a.Status, nil
		}
		if time.Now().After(deadline) {
			// Mark expired on the way out so retries see the right state.
			now := time.Now().UTC()
			a.Status = approval.StatusExpired
			a.DecidedAt = &now
			expiredBy := "system:timeout"
			a.DecidedBy = &expiredBy
			if err := g.repo.Update(ctx, a); err != nil {
				g.logger.Warn("approval: mark-expired update failed",
					"id", a.ID, "err", err)
			}
			return approval.StatusExpired, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-t.C:
		}
	}
}

// Notify is a no-op for R3.5b (humans act via CLI). R3.x adds webhook
// callbacks here.
func (g *Gate) Notify(ctx context.Context, approvalID string, status approval.Status, decidedBy string) error {
	if status != approval.StatusGranted && status != approval.StatusDenied {
		return errors.New("approval: notify only accepts granted|denied")
	}
	_, err := g.repo.Get(ctx, approvalID)
	if err != nil {
		return err
	}
	if decidedBy == "" {
		return errors.New("approval: notify requires decided_by")
	}
	// R3.5b: hand off to the service-layer Grant/Deny. R3.x: extend
	// the Repo to take status + decided_by directly.
	if status == approval.StatusGranted {
		_, err = (&Service{repo: g.repo}).Grant(ctx, approvalID, decidedBy)
	} else {
		_, err = (&Service{repo: g.repo}).Deny(ctx, approvalID, decidedBy)
	}
	return err
}

// _ ensures Gate implements approval.Gate.
var _ approval.Gate = (*Gate)(nil)