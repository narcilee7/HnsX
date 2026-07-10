package spec

import "sort"

// DeriveCapabilities returns the capability tags implied by a DomainSpec.
// These tags are used by the scheduler to match sessions against workers.
//
// Capability format follows "namespace:value" conventions:
//   - provider:<provider>        e.g. provider:anthropic
//   - model:<model>              e.g. model:claude-haiku-4-5
//   - adapter:<kind>             e.g. adapter:anthropic
//   - sandbox:<policy>           e.g. sandbox:process
//   - tool:<name>                e.g. tool:web_search
//   - tool_kind:<kind>           e.g. tool_kind:http
//
// A worker declares the capabilities it offers via WorkerInfo.Capacity;
// the scheduler pulls only sessions whose required capabilities are a
// subset of the worker's offered capabilities.
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

	for name, tool := range s.Harness.Tools {
		if name != "" {
			caps["tool:"+name] = struct{}{}
		}
		if tool.Kind != "" {
			caps["tool_kind:"+tool.Kind] = struct{}{}
		}
	}

	out := make([]string, 0, len(caps))
	for c := range caps {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
