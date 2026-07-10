// Package scorer implements rule-based scorers used by the eval runner to
// compare a session's actual output against a case's expectation.
//
// Scorer kinds:
//   - "exact" / "equals" / "json_match": every key in expect must match actual.
//   - "contains": every key in expect must appear (as a substring) in actual.
//   - "llm_judge": placeholder — fails closed until a real adapter is wired.
//
// An empty scorer type defaults to "exact". Unknown kinds default to "exact"
// with a warning recorded in the verdict details.
package scorer

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
)

// Verdict is the outcome of scoring one case.
type Verdict struct {
	Score   float64
	Passed  bool
	Details map[string]any
}

// Score evaluates actual output against expect using the scorer configuration.
// Unavailable/unknown scorers fail closed (Passed=false where applicable).
func Score(sc model.Scorer, expect, actual map[string]any) Verdict {
	kind := strings.ToLower(strings.TrimSpace(sc.Type))
	if kind == "" {
		kind = "exact"
	}
	switch kind {
	case "exact", "equals", "json_match":
		return matchKeys(expect, actual, valuesEqual, sc.Config)
	case "contains":
		return matchKeys(expect, actual, valueContains, sc.Config)
	case "llm_judge", "llm":
		return Verdict{Score: 0, Passed: false, Details: map[string]any{
			"scorer": kind,
			"error":  "llm_judge scorer not available in this build",
		}}
	default:
		v := matchKeys(expect, actual, valuesEqual, sc.Config)
		if v.Details == nil {
			v.Details = map[string]any{}
		}
		v.Details["warning"] = fmt.Sprintf("unknown scorer %q, defaulted to exact match", kind)
		return v
	}
}

// matchKeys scores the fraction of expect keys satisfied by actual via cmp.
// A "threshold" (0..1) in cfg overrides the default all-or-nothing pass bar.
func matchKeys(expect, actual map[string]any, cmp func(got, want any) bool, cfg map[string]any) Verdict {
	if len(expect) == 0 {
		return Verdict{Score: 1, Passed: true, Details: map[string]any{
			"note": "no expectation; vacuously passed",
		}}
	}
	matched := 0
	mismatches := map[string]any{}
	for k, want := range expect {
		got, ok := actual[k]
		if ok && cmp(got, want) {
			matched++
		} else {
			mismatches[k] = map[string]any{"expected": want, "actual": got}
		}
	}
	score := float64(matched) / float64(len(expect))
	threshold := 1.0
	if t, ok := floatFromConfig(cfg, "threshold"); ok {
		threshold = t
	}
	details := map[string]any{"matched": matched, "total": len(expect)}
	if len(mismatches) > 0 {
		details["mismatches"] = mismatches
	}
	return Verdict{Score: score, Passed: score >= threshold, Details: details}
}

func valuesEqual(got, want any) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	// JSON round-trips numbers to float64 and everything stringifies cleanly, so
	// fall back to a string comparison for cross-type equality (e.g. 1 vs 1.0).
	return fmt.Sprintf("%v", got) == fmt.Sprintf("%v", want)
}

func valueContains(got, want any) bool {
	return strings.Contains(fmt.Sprintf("%v", got), fmt.Sprintf("%v", want))
}

func floatFromConfig(cfg map[string]any, key string) (float64, bool) {
	if cfg == nil {
		return 0, false
	}
	v, ok := cfg[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
