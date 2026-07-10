package scorer

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
)

func TestScore_Exact(t *testing.T) {
	sc := model.Scorer{Type: "exact"}
	v := Score(sc, map[string]any{"answer": "yes"}, map[string]any{"answer": "yes", "extra": 1})
	if !v.Passed || v.Score != 1 {
		t.Fatalf("expected pass score 1, got passed=%v score=%v", v.Passed, v.Score)
	}

	v = Score(sc, map[string]any{"answer": "yes"}, map[string]any{"answer": "no"})
	if v.Passed || v.Score != 0 {
		t.Fatalf("expected fail score 0, got passed=%v score=%v", v.Passed, v.Score)
	}
}

func TestScore_EmptyDefaultsToExact(t *testing.T) {
	v := Score(model.Scorer{}, map[string]any{"k": "v"}, map[string]any{"k": "v"})
	if !v.Passed {
		t.Fatalf("empty scorer should default to exact and pass")
	}
}

func TestScore_Contains(t *testing.T) {
	sc := model.Scorer{Type: "contains"}
	v := Score(sc, map[string]any{"response": "double"}, map[string]any{"response": "you were charged double this month"})
	if !v.Passed {
		t.Fatalf("contains should pass when substring present: %+v", v)
	}

	v = Score(sc, map[string]any{"response": "refund"}, map[string]any{"response": "no match here"})
	if v.Passed {
		t.Fatalf("contains should fail when substring absent")
	}
}

func TestScore_PartialWithThreshold(t *testing.T) {
	sc := model.Scorer{Type: "exact", Config: map[string]any{"threshold": 0.5}}
	expect := map[string]any{"a": 1, "b": 2}
	actual := map[string]any{"a": 1, "b": 999}
	v := Score(sc, expect, actual)
	if v.Score != 0.5 {
		t.Fatalf("expected score 0.5, got %v", v.Score)
	}
	if !v.Passed {
		t.Fatalf("score 0.5 should pass with threshold 0.5")
	}
}

func TestScore_NumberCrossType(t *testing.T) {
	// JSON decodes numbers to float64; an int expectation should still match.
	v := Score(model.Scorer{Type: "exact"}, map[string]any{"n": 1}, map[string]any{"n": 1.0})
	if !v.Passed {
		t.Fatalf("1 vs 1.0 should match via string fallback")
	}
}

func TestScore_LLMJudgeUnavailable(t *testing.T) {
	v := Score(model.Scorer{Type: "llm_judge"}, map[string]any{"k": "v"}, map[string]any{"k": "v"})
	if v.Passed {
		t.Fatalf("llm_judge must fail closed in this build")
	}
	if v.Details["error"] == nil {
		t.Fatalf("expected an error detail for llm_judge")
	}
}
