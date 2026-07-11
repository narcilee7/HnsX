package spec

// PresetName is a named policy bundle that expands into concrete budget,
// permission, approval and guardrail settings.
type PresetName string

const (
	// SafeCustomerService blocks destructive side-effects and requires human
	// approval for refunds or customer-data exports.
	SafeCustomerService PresetName = "safe_customer_service"
)

// PresetRegistry maps preset names to their expanded PolicySpec. Presets are
// merged in order; explicit user values override preset defaults.
var PresetRegistry = map[PresetName]PolicySpec{
	SafeCustomerService: {
		Budget: BudgetSpec{
			MaxCostUSD: 1.0,
			MaxTurns:   20,
		},
		Permissions: PermissionSpec{
			AllowNetwork:    true,
			AllowFileWrite:  false,
			AllowFileDelete: false,
			AllowShell:      false,
		},
		Approval: ApprovalSpec{
			DefaultTimeoutSeconds: 600,
			RequiredFor: RequiredForSpec{
				Tools:     []string{"issue_refund", "export_customer_data"},
				Resources: []string{"billing:write", "customer:*"},
			},
		},
		Guardrails: []GuardrailSpec{
			{
				ID:      "block-pii-keywords",
				Type:    "output_keyword",
				On:      "output",
				Action:  "block",
				Message: "output contains potential PII",
				Config:  map[string]any{"keywords": []string{"password", "credit card", "ssn"}},
			},
		},
	},
}

// ExpandPresets returns a new PolicySpec with all named presets merged into
// the base spec. User-provided fields take precedence over preset defaults.
func ExpandPresets(base PolicySpec) PolicySpec {
	out := base
	for _, name := range base.Presets {
		preset, ok := PresetRegistry[PresetName(name)]
		if !ok {
			continue
		}
		mergePreset(&out, preset)
	}
	return out
}

func mergePreset(dst *PolicySpec, src PolicySpec) {
	if dst.Budget.MaxCostUSD == 0 {
		dst.Budget.MaxCostUSD = src.Budget.MaxCostUSD
	}
	if dst.Budget.MaxTurns == 0 {
		dst.Budget.MaxTurns = src.Budget.MaxTurns
	}
	if dst.Budget.MaxTokens == 0 {
		dst.Budget.MaxTokens = src.Budget.MaxTokens
	}

	// Permission booleans: explicit false is meaningful, so only override when
	// the dst value is the zero value (false). In practice this means a preset
	// can turn a capability ON, but turning it OFF requires an explicit user
	// value; this matches the "safe" preset semantics.
	if !dst.Permissions.AllowFileWrite && src.Permissions.AllowFileWrite {
		dst.Permissions.AllowFileWrite = true
	}
	if !dst.Permissions.AllowFileDelete && src.Permissions.AllowFileDelete {
		dst.Permissions.AllowFileDelete = true
	}
	if !dst.Permissions.AllowNetwork && src.Permissions.AllowNetwork {
		dst.Permissions.AllowNetwork = true
	}
	if !dst.Permissions.AllowShell && src.Permissions.AllowShell {
		dst.Permissions.AllowShell = true
	}

	if dst.Approval.DefaultTimeoutSeconds == 0 {
		dst.Approval.DefaultTimeoutSeconds = src.Approval.DefaultTimeoutSeconds
	}
	dst.Approval.RequiredFor.Tools = mergeUnique(dst.Approval.RequiredFor.Tools, src.Approval.RequiredFor.Tools)
	dst.Approval.RequiredFor.Resources = mergeUnique(dst.Approval.RequiredFor.Resources, src.Approval.RequiredFor.Resources)
	if dst.Approval.RequiredFor.CostThresholdUSD == 0 {
		dst.Approval.RequiredFor.CostThresholdUSD = src.Approval.RequiredFor.CostThresholdUSD
	}

	dst.Guardrails = append(dst.Guardrails, src.Guardrails...)
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range b {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// KnownPresets returns every registered preset name.
func KnownPresets() []string {
	out := make([]string, 0, len(PresetRegistry))
	for name := range PresetRegistry {
		out = append(out, string(name))
	}
	return out
}
