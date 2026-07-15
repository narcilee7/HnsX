package engine

import (
	"context"
	"testing"
)

func TestFlatPolicy_NoPolicy(t *testing.T) {
	var p *FlatPolicy
	d := p.Check(context.Background(), Action{EstimatedCostUSD: 100})
	if !d.Allow {
		t.Fatalf("expected allow for nil policy; got %+v", d)
	}
}

func TestFlatPolicy_CostGate(t *testing.T) {
	p := &FlatPolicy{MaxCostUSD: 1.0}
	d := p.Check(context.Background(), Action{EstimatedCostUSD: 0.5})
	if !d.Allow {
		t.Fatalf("expected allow for cost under threshold; got %+v", d)
	}
	d = p.Check(context.Background(), Action{EstimatedCostUSD: 5.0})
	if !d.RequireApproval {
		t.Fatalf("expected require approval for cost over threshold; got %+v", d)
	}
}

func TestFlatPolicy_ResourceDeny(t *testing.T) {
	p := &FlatPolicy{BlockedResources: []string{"rm -rf /"}}
	d := p.Check(context.Background(), Action{Resource: "rm -rf /"})
	if d.Allow {
		t.Fatalf("expected deny for blocked resource; got %+v", d)
	}
}
