package spec

import "sort"

// DeriveCapabilities returns the capability tags implied by a DomainSpec.
// These tags are used by the scheduler to match sessions against workers.
//
// Capability format follows "namespace:value" conventions:
//   - provider:<provider>        e.g. provider:anthropic
//   - model:<model>              e.g. model:claude-haiku-4-5
//   - adapter:<kind>             e.g. adapter:echo
//   - sandbox:<policy>           e.g. sandbox:none
//
// Tool capabilities are intentionally omitted until workers advertise the
// tool sets they support; the Python runtime currently ships with built-in
// tools and resolves them at execution time.
func DeriveCapabilities(s *DomainSpec) []string {
	if s == nil {
		return nil
	}
	caps := map[string]struct{}{}

	for _, agent := range s.Harness.Agents {
		if agent.Provider != "" {
			caps["provider:"+agent.Provider] = struct{}{}
		}
		if agent.Model != "" {
			caps["model:"+agent.Model] = struct{}{}
		}
		if agent.Adapter.Kind != "" {
			caps["adapter:"+agent.Adapter.Kind] = struct{}{}
		}
	}

	if s.Harness.Sandbox.Policy != "" {
		caps["sandbox:"+s.Harness.Sandbox.Policy] = struct{}{}
	}

	// Tool capabilities intentionally omitted — see doc comment above.

	out := make([]string, 0, len(caps))
	for c := range caps {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
