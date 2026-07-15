package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/cli"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

func TestScoreMatch(t *testing.T) {
	cases := map[string]struct {
		actual, expect string
		want           float64
	}{
		"empty expect": {"hello world", "", 1.0},
		"all match":    {"foo bar baz", "foo bar baz", 1.0},
		"partial":      {"foo bar", "foo baz", 0.5},
		"empty actual": {"", "foo", 0.0},
		"case sens":    {"FOO", "foo", 0.0},
	}
	for name, c := range cases {
		if got := scoreMatch(c.actual, c.expect); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func TestEvalRunner_RunAllPass(t *testing.T) {
	r := NewEvalRunner(&FlatPolicy{}, func(ctx context.Context, inv cli.Invocation) error {
		// Simulate an agent whose output matches the expectation.
		inv.OnMessage(wire.TaskMessage{Type: "text", Content: "result: pass"})
		return nil
	}, ExecutorDefaults{Command: "fake", Args: []string{"-p"}, EstimatedCostUSD: 0.1})

	set := EvalSet{
		ID:       "set-1",
		DomainID: "d-1",
		Cases: []EvalCase{
			{ID: "c1", Input: "q1", Expect: "pass"},
			{ID: "c2", Input: "q2", Expect: "pass"},
		},
	}
	rep := r.Run(context.Background(), set, nil)
	if rep.Score != 1.0 {
		t.Errorf("Score = %v, want 1.0", rep.Score)
	}
	if rep.PassRate != 1.0 {
		t.Errorf("PassRate = %v, want 1.0", rep.PassRate)
	}
	if rep.TotalCostUSD != 0.2 {
		t.Errorf("TotalCostUSD = %v, want 0.2", rep.TotalCostUSD)
	}
	if len(rep.Cases) != 2 {
		t.Errorf("expected 2 case results; got %d", len(rep.Cases))
	}
}

func TestEvalRunner_RunPartialFail(t *testing.T) {
	r := NewEvalRunner(&FlatPolicy{}, func(ctx context.Context, inv cli.Invocation) error {
		inv.OnMessage(wire.TaskMessage{Type: "text", Content: "fail"})
		return nil
	}, ExecutorDefaults{Command: "fake"})
	set := EvalSet{
		ID: "set-2",
		Cases: []EvalCase{
			{ID: "c1", Expect: "pass"},
			{ID: "c2", Expect: "fail"},
		},
	}
	rep := r.Run(context.Background(), set, nil)
	if rep.PassRate != 0.5 {
		t.Errorf("PassRate = %v, want 0.5", rep.PassRate)
	}
	if rep.Score != 0.5 {
		t.Errorf("Score = %v, want 0.5", rep.Score)
	}
}

func TestEvalRunner_RegressionDetected(t *testing.T) {
	r := NewEvalRunner(&FlatPolicy{}, func(ctx context.Context, inv cli.Invocation) error {
		inv.OnMessage(wire.TaskMessage{Type: "text", Content: "nope"})
		return nil
	}, ExecutorDefaults{Command: "fake"})
	baseline := &EvalReport{EvalSetID: "baseline", Score: 0.9}
	set := EvalSet{ID: "set-3", Cases: []EvalCase{
		{ID: "c1", Expect: "pass"},
	}}
	rep := r.Run(context.Background(), set, baseline)
	if !rep.Regressed {
		t.Errorf("expected Regressed=true; got false (score=%v baseline=%v)", rep.Score, baseline.Score)
	}
}

func TestEvalRunner_SubprocessError(t *testing.T) {
	r := NewEvalRunner(&FlatPolicy{}, func(ctx context.Context, inv cli.Invocation) error {
		return context.DeadlineExceeded
	}, ExecutorDefaults{Command: "fake"})
	set := EvalSet{ID: "set-4", Cases: []EvalCase{
		{ID: "c1", Expect: "pass"},
	}}
	rep := r.Run(context.Background(), set, nil)
	if rep.PassRate != 0.0 {
		t.Errorf("expected PassRate=0; got %v", rep.PassRate)
	}
	if !strings.Contains(rep.Cases[0].FailureReason, "deadline") {
		t.Errorf("expected deadline in failure; got %q", rep.Cases[0].FailureReason)
	}
}
