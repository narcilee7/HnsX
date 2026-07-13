package runner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	domainrepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrepo "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	sessionrepo "github.com/hnsx-io/hnsx/server/internal/session/repository"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	workerrepo "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func TestWorkerPoolRunner_ScoresAndAggregates(t *testing.T) {
	oldPoll := pollInterval
	pollInterval = 20 * time.Millisecond
	defer func() { pollInterval = oldPoll }()

	evalSvc, sessionSvc, sessionCmds, workerSvc := newWorkerPoolTestDeps(t)

	set := &evalmodel.EvalSet{
		ID:       "wp-set1",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Input: map[string]any{"produce": "yes"}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
			{ID: "c2", Input: map[string]any{"produce": "no"}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
		},
	}
	run := newRun(t, evalSvc, set)

	stop := make(chan struct{})
	defer close(stop)
	go driveWorkerPool(t, workerSvc, sessionSvc, stop, func(trigger map[string]any) *domain.Result {
		return &domain.Result{State: "completed", Output: map[string]any{"answer": trigger["produce"]}}
	})

	r := NewWorkerPoolRunner(sessionCmds, sessionSvc, evalSvc, nil)
	if err := r.Run(t.Context(), run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, err := evalSvc.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.State != "completed" {
		t.Fatalf("state = %q, want completed", got.State)
	}
	if got.PassedCases != 1 || got.TotalCases != 2 {
		t.Fatalf("passed=%d total=%d, want 1/2", got.PassedCases, got.TotalCases)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got.Results))
	}
}

func TestWorkerPoolRunner_FailedSessionScoresZero(t *testing.T) {
	oldPoll := pollInterval
	pollInterval = 20 * time.Millisecond
	defer func() { pollInterval = oldPoll }()

	evalSvc, sessionSvc, sessionCmds, workerSvc := newWorkerPoolTestDeps(t)

	set := &evalmodel.EvalSet{
		ID:       "wp-set2",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Input: map[string]any{"fail": false}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
			{ID: "c2", Input: map[string]any{"fail": true}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
		},
	}
	run := newRun(t, evalSvc, set)

	stop := make(chan struct{})
	defer close(stop)
	go driveWorkerPool(t, workerSvc, sessionSvc, stop, func(trigger map[string]any) *domain.Result {
		if trigger["fail"].(bool) {
			return nil
		}
		return &domain.Result{State: "completed", Output: map[string]any{"answer": "yes"}}
	})

	r := NewWorkerPoolRunner(sessionCmds, sessionSvc, evalSvc, nil)
	if err := r.Run(t.Context(), run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, _ := evalSvc.GetRun(run.ID)
	if got.PassedCases != 1 || got.Score != 0.5 {
		t.Fatalf("passed=%d score=%v, want 1/0.5", got.PassedCases, got.Score)
	}
}

func TestWorkerPoolRunner_RequiresSessionCommands(t *testing.T) {
	evalSvc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	sessionSvc := sessionservice.NewService(sessionrepo.NewInMemoryRepository())
	r := NewWorkerPoolRunner(nil, sessionSvc, evalSvc, nil)
	if err := r.Run(t.Context(), &evalmodel.EvalRun{ID: "r", DomainID: "d1"}, &evalmodel.EvalSet{ID: "s", DomainID: "d1"}, &domain.DomainSpec{ID: "d1"}, 0); err == nil {
		t.Fatal("expected error for missing session commands")
	}
}

func TestWorkerPoolRunner_RequiresSessionService(t *testing.T) {
	evalSvc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	sessionSvc := sessionservice.NewService(sessionrepo.NewInMemoryRepository())
	domainSvc := domainservice.NewService(domainrepo.NewInMemoryRepository())
	workerSvc := workerservice.NewService(workerrepo.NewInMemoryRepository())
	cmds := commands.NewSessionCommands(sessionSvc, domainSvc, workerSvc, app.NewState())
	r := NewWorkerPoolRunner(cmds, nil, evalSvc, nil)
	if err := r.Run(t.Context(), &evalmodel.EvalRun{ID: "r", DomainID: "d1"}, &evalmodel.EvalSet{ID: "s", DomainID: "d1"}, &domain.DomainSpec{ID: "d1"}, 0); err == nil {
		t.Fatal("expected error for missing session service")
	}
}

func TestWorkerPoolRunner_UsesTenantFromContext(t *testing.T) {
	oldPoll := pollInterval
	pollInterval = 20 * time.Millisecond
	defer func() { pollInterval = oldPoll }()

	evalSvc, sessionSvc, sessionCmds, workerSvc := newWorkerPoolTestDeps(t)

	set := &evalmodel.EvalSet{
		ID:       "wp-tenant",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Input: map[string]any{"produce": "yes"}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
		},
	}
	run := newRun(t, evalSvc, set)

	stop := make(chan struct{})
	defer close(stop)
	go driveWorkerPool(t, workerSvc, sessionSvc, stop, func(trigger map[string]any) *domain.Result {
		return &domain.Result{State: "completed", Output: map[string]any{"answer": trigger["produce"]}}
	})

	r := NewWorkerPoolRunner(sessionCmds, sessionSvc, evalSvc, nil)
	ctx := tenant.NewContext(t.Context(), tenant.ID("t-42"))
	if err := r.Run(ctx, run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, _ := evalSvc.GetRun(run.ID)
	if got.State != "completed" {
		t.Fatalf("state = %q, want completed", got.State)
	}
}

func TestWorkerPoolRunner_CreatesSessions(t *testing.T) {
	oldPoll := pollInterval
	pollInterval = 20 * time.Millisecond
	defer func() { pollInterval = oldPoll }()

	evalSvc, sessionSvc, sessionCmds, workerSvc := newWorkerPoolTestDeps(t)

	set := &evalmodel.EvalSet{
		ID:       "wp-sessions",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Input: map[string]any{}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
		},
	}
	run := newRun(t, evalSvc, set)

	stop := make(chan struct{})
	defer close(stop)
	go driveWorkerPool(t, workerSvc, sessionSvc, stop, func(map[string]any) *domain.Result {
		return &domain.Result{State: "completed", Output: map[string]any{"answer": "yes"}}
	})

	r := NewWorkerPoolRunner(sessionCmds, sessionSvc, evalSvc, nil)
	if err := r.Run(t.Context(), run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}

	list, _ := sessionSvc.List(tenant.DefaultID)
	if len(list) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list))
	}
}

func newWorkerPoolTestDeps(t *testing.T) (*evalservice.Service, *sessionservice.Service, *commands.SessionCommands, *workerservice.Service) {
	t.Helper()
	evalSvc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	sessionSvc := sessionservice.NewService(sessionrepo.NewInMemoryRepository())
	domainSvc := domainservice.NewService(domainrepo.NewInMemoryRepository())
	workerSvc := workerservice.NewService(workerrepo.NewInMemoryRepository())
	state := app.NewState()
	cmds := commands.NewSessionCommands(sessionSvc, domainSvc, workerSvc, state)
	return evalSvc, sessionSvc, cmds, workerSvc
}

func newRun(t *testing.T, evalSvc *evalservice.Service, set *evalmodel.EvalSet) *evalmodel.EvalRun {
	t.Helper()
	run := &evalmodel.EvalRun{
		ID:         uuid.NewString(),
		EvalSetID:  set.ID,
		DomainID:   set.DomainID,
		State:      "running",
		TotalCases: len(set.Cases),
	}
	if err := evalSvc.CreateRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

// driveWorkerPool pulls sessions from the worker queue and completes them using
// the supplied result factory. It stops when the channel is closed.
func driveWorkerPool(t *testing.T, workerSvc *workerservice.Service, sessionSvc *sessionservice.Service, stop <-chan struct{}, factory func(map[string]any) *domain.Result) {
	t.Helper()
	for {
		select {
		case <-stop:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		req, ok := workerSvc.DequeueSession(ctx, nil)
		cancel()
		if !ok {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		_, _ = sessionSvc.MarkRunning(tenant.DefaultID, req.SessionID)
		trigger := map[string]any{}
		_ = json.Unmarshal([]byte(req.TriggerPayloadJSON), &trigger)
		res := factory(trigger)
		if res == nil {
			_, _ = sessionSvc.MarkFailed(tenant.DefaultID, req.SessionID)
		} else {
			_, _ = sessionSvc.MarkCompleted(tenant.DefaultID, req.SessionID, res)
		}
	}
}

// Ensure the interface is satisfied by the worker pool runner.
var _ EvalRunner = (*WorkerPoolRunner)(nil)
