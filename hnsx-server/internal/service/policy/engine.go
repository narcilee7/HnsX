// Package policy hosts the application-level orchestration for Policy.
// The Engine is the runtime evaluator: given a Policy and an
// EvalContext, it walks the rules in priority order and returns the
// first matching Action (or allow, the default).
package policy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
)

// ErrInvalidExpression is returned by the engine when a rule's
// expression cannot be parsed. We surface it as 400 at the HTTP layer.
var ErrInvalidExpression = errors.New("policy: invalid expression")

// Engine is the production engine. It supports a tiny expression
// subset suitable for cost ceilings and token guards; real JMESPath
// integration lands in R3.x.
type Engine struct{}

// NewEngine constructs the production engine.
func NewEngine() *Engine { return &Engine{} }

// Evaluate walks p.Rules in priority order and returns the first
// matching Decision. If no rule matches, returns Decision{Allow}.
//
// Supported expression grammar (kept tiny for R3):
//
//   cost_usd > 0.50
//   tokens_in > 1000
//   tokens_out > 500
//   total_cost > 1.00          (alias for cost_usd)
//   tool_name == "Bash"
//   tool_name in ["Bash","Write"]
//
// Operators: >, <, >=, <=, ==, !=, in
// Logical:   &&, ||
func (e *Engine) Evaluate(ctx context.Context, p *policy.Policy, ec policy.EvalContext) (policy.Decision, error) {
	rules, err := p.RulesTyped()
	if err != nil {
		return policy.Decision{}, fmt.Errorf("policy: decode rules: %w", err)
	}
	for _, r := range rules {
		match, err := evalExpr(r.Expression, ec)
		if err != nil {
			return policy.Decision{}, fmt.Errorf("rule %s: %w", r.ID, ErrInvalidExpression)
		}
		if match {
			return policy.Decision{Action: r.Action, RuleID: r.ID, Message: r.Message}, nil
		}
	}
	return policy.Decision{Action: policy.ActionAllow}, nil
}

// evalExpr returns whether the expression matches against ec.
func evalExpr(expr string, ec policy.EvalContext) (bool, error) {
	expr = strings.TrimSpace(expr)
	// OR
	if idx := strings.Index(expr, "||"); idx > 0 {
		l, err := evalExpr(expr[:idx], ec)
		if err != nil {
			return false, err
		}
		if l {
			return true, nil
		}
		return evalExpr(expr[idx+2:], ec)
	}
	// AND
	if idx := strings.Index(expr, "&&"); idx > 0 {
		l, err := evalExpr(expr[:idx], ec)
		if err != nil {
			return false, err
		}
		if !l {
			return false, nil
		}
		return evalExpr(expr[idx+2:], ec)
	}
	// leaf: <lhs> <op> <rhs>
	lhs, op, rhs, ok := splitLeaf(expr)
	if !ok {
		return false, fmt.Errorf("unparseable expression: %q", expr)
	}
	lv, err := lookupField(lhs, ec)
	if err != nil {
		return false, err
	}
	rv, err := parseLiteral(rhs)
	if err != nil {
		return false, err
	}
	return compare(lv, op, rv)
}

// splitLeaf splits "lhs op rhs" — lhs is an identifier, rhs is a literal.
// We tokenize by whitespace + operator so "name in [..]" parses correctly
// (without "in" inside "name" being mistaken for the operator).
func splitLeaf(expr string) (lhs, op, rhs string, ok bool) {
	// regex: <lhs> <op> <rhs>
	// ops ordered longest-first so ">=" doesn't match ">"
	ops := []string{">=", "<=", "==", "!=", "in", ">", "<"}
	for _, o := range ops {
		// match "<lhs> <op> <rhs>" with mandatory whitespace around op
		pattern := " " + o + " "
		if idx := strings.Index(expr, pattern); idx > 0 {
			return strings.TrimSpace(expr[:idx]), o, strings.TrimSpace(expr[idx+len(pattern):]), true
		}
	}
	return "", "", "", false
}

// isIdentChar is kept for any future tokenization extensions; the
// current splitLeaf uses whitespace-bound operators instead.
var _ = func(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '.'
}

// lookupField resolves a dotted identifier (cost_usd / tokens_in /
// total_cost / tool_name / agent_id / workspace_id / issue_id) against
// the EvalContext. Returns a generic value suitable for compare().
func lookupField(name string, ec policy.EvalContext) (any, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cost_usd", "total_cost":
		return ec.CostUSD, nil
	case "tokens_in":
		return ec.TokensIn, nil
	case "tokens_out":
		return ec.TokensOut, nil
	case "tool_name":
		return ec.ToolName, nil
	case "agent_id":
		return ec.AgentID, nil
	case "workspace_id":
		return ec.WorkspaceID, nil
	case "issue_id":
		return ec.IssueID, nil
	case "action":
		return ec.Action, nil
	}
	return nil, fmt.Errorf("unknown field %q", name)
}

// parseLiteral handles numbers, strings, and lists.
func parseLiteral(s string) (any, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return s[1 : len(s)-1], nil
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []any{}, nil
		}
		var out []any
		for _, part := range strings.Split(inner, ",") {
			v, err := parseLiteral(part)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	return s, nil
}

// compare applies op to numeric or string operands. For "in", rhs must
// be a list (any element equal to lhs yields true).
func compare(lhs any, op string, rhs any) (bool, error) {
	if op == "in" {
		list, ok := rhs.([]any)
		if !ok {
			return false, fmt.Errorf("'in' expects a list, got %T", rhs)
		}
		ls := fmt.Sprintf("%v", lhs)
		for _, item := range list {
			if fmt.Sprintf("%v", item) == ls {
				return true, nil
			}
		}
		return false, nil
	}

	// Numeric compare when both sides parse as floats.
	lf, lok := toFloat(lhs)
	rf, rok := toFloat(rhs)
	if lok && rok {
		switch op {
		case ">":
			return lf > rf, nil
		case "<":
			return lf < rf, nil
		case ">=":
			return lf >= rf, nil
		case "<=":
			return lf <= rf, nil
		case "==":
			return lf == rf, nil
		case "!=":
			return lf != rf, nil
		}
	}

	// Fallback: string compare.
	ls := fmt.Sprintf("%v", lhs)
	rs := fmt.Sprintf("%v", rhs)
	switch op {
	case "==":
		return ls == rs, nil
	case "!=":
		return ls != rs, nil
	}
	return false, fmt.Errorf("unsupported op %q for non-numeric operands", op)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// _ guards compile-time conformance.
var _ policy.Engine = (*Engine)(nil)