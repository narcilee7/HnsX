// Package spec defines the Harness-first DomainSpec v2 and its sub-types.
//
// This file adds bidirectional conversion between the Go-native DomainSpec
// (used by the loader, CLI, and REST API) and the protobuf DomainSpec
// (used by the Connect control plane). The two schemas diverge intentionally:
// the Go spec uses maps and dynamic `any` fields for authoring ergonomics,
// while the proto spec uses repeated messages and JSON-string fields for a
// stable wire format. Conversion therefore serializes dynamic blocks to JSON
// strings where the proto expects them, and materializes them back on the
// reverse path.
package spec

import (
	"encoding/json"
	"fmt"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// ToProto converts the Go-native DomainSpec into its protobuf representation.
func ToProto(s *DomainSpec) (*pb.DomainSpec, error) {
	if s == nil {
		return nil, nil
	}
	out := &pb.DomainSpec{
		Id:          s.ID,
		Version:     s.Version,
		Description: s.Description,
	}
	harness, err := harnessToProto(&s.Harness)
	if err != nil {
		return nil, fmt.Errorf("harness: %w", err)
	}
	out.Harness = harness
	return out, nil
}

// FromProto converts a protobuf DomainSpec back into the Go-native form.
func FromProto(p *pb.DomainSpec) (*DomainSpec, error) {
	if p == nil {
		return nil, nil
	}
	out := &DomainSpec{
		ID:          p.GetId(),
		Version:     p.GetVersion(),
		Description: p.GetDescription(),
	}
	harness, err := harnessFromProto(p.GetHarness())
	if err != nil {
		return nil, fmt.Errorf("harness: %w", err)
	}
	out.Harness = *harness
	return out, nil
}

func harnessToProto(h *HarnessSpec) (*pb.Harness, error) {
	if h == nil {
		return nil, nil
	}
	out := &pb.Harness{}
	for id, a := range h.Agents {
		out.Agents = append(out.Agents, agentToProto(id, a))
	}
	for id, p := range h.Prompts {
		out.Prompts = append(out.Prompts, &pb.Prompt{
			Id:        id,
			Template:  p.Template,
			Variables: stringifyMap(p.Schema),
		})
	}
	for id, s := range h.Skills {
		out.Skills = append(out.Skills, skillToProto(id, s))
	}
	for id, t := range h.Tools {
		cfg, err := toJSONString(t.Config)
		if err != nil {
			return nil, fmt.Errorf("tool %q config: %w", id, err)
		}
		out.Tools = append(out.Tools, &pb.Tool{
			Id:          id,
			Description: t.Description,
			Type:        t.Kind,
			Config:      cfg,
		})
	}
	if h.MCP != nil {
		for _, m := range h.MCP.Servers {
			out.Mcps = append(out.Mcps, &pb.MCPConfig{
				Id:      m.Name,
				Command: m.Command,
				Args:    m.Args,
				Env:     m.Headers,
			})
		}
	}
	if h.Sandbox.Policy != "" || h.Sandbox.Runtime != "" || h.Sandbox.Filesystem != nil || h.Sandbox.Network != nil || h.Sandbox.Commands != nil || h.Sandbox.Resources != nil {
		cfg, err := toJSONString(h.Sandbox)
		if err != nil {
			return nil, fmt.Errorf("sandbox config: %w", err)
		}
		out.Sandbox = &pb.Sandbox{
			Backend: h.Sandbox.Policy,
			Config:  cfg,
		}
	}
	if h.Policy.Budget.MaxCostUSD != 0 || !h.Policy.Permissions.IsZero() || len(h.Policy.Guardrails) > 0 {
		grd, err := toJSONString(h.Policy.Guardrails)
		if err != nil {
			return nil, fmt.Errorf("policy guardrails: %w", err)
		}
		out.Policy = &pb.Policy{
			BudgetUsd:            h.Policy.Budget.MaxCostUSD,
			AllowedTools:         allowedTools(h.Policy.Guardrails),
			DeniedTools:          deniedTools(h.Policy.Guardrails),
			RequireHumanApproval: requiresApproval(h.Policy.Guardrails),
			Guardrails:           grd,
		}
	}
	if h.Store != nil {
		cfg, err := toJSONString(h.Store)
		if err != nil {
			return nil, fmt.Errorf("memory config: %w", err)
		}
		out.Memory = &pb.Memory{
			Backend: "store",
			Config:  cfg,
		}
	}
	if h.Telemetry != nil {
		cfg, err := toJSONString(h.Telemetry)
		if err != nil {
			return nil, fmt.Errorf("telemetry config: %w", err)
		}
		if out.Memory == nil {
			out.Memory = &pb.Memory{}
		}
		out.Memory.Config = cfg
	}
	out.Session = sessionToProto(&h.Session)
	return out, nil
}

func harnessFromProto(h *pb.Harness) (*HarnessSpec, error) {
	if h == nil {
		return &HarnessSpec{}, nil
	}
	out := &HarnessSpec{
		Agents:  make(map[string]AgentSpec),
		Prompts: make(map[string]PromptSpec),
		Skills:  make(map[string]SkillSpec),
		Tools:   make(map[string]ToolConfig),
	}
	for _, a := range h.GetAgents() {
		id, spec := agentFromProto(a)
		out.Agents[id] = spec
	}
	for _, p := range h.GetPrompts() {
		out.Prompts[p.GetId()] = PromptSpec{
			Type:     "text",
			Template: p.GetTemplate(),
			Schema:   mapFromStrings(p.GetVariables()),
		}
	}
	for _, s := range h.GetSkills() {
		id, spec := skillFromProto(s)
		out.Skills[id] = spec
	}
	for _, t := range h.GetTools() {
		var cfg any
		if t.GetConfig() != "" {
			if err := json.Unmarshal([]byte(t.GetConfig()), &cfg); err != nil {
				return nil, fmt.Errorf("tool %q config: %w", t.GetId(), err)
			}
		}
		out.Tools[t.GetId()] = ToolConfig{
			Kind:        t.GetType(),
			Name:        t.GetId(),
			Description: t.GetDescription(),
			Config:      cfg,
		}
	}
	if len(h.GetMcps()) > 0 {
		mcp := &MCPConfig{}
		for _, m := range h.GetMcps() {
			mcp.Servers = append(mcp.Servers, MCPServer{
				Name:      m.GetId(),
				Transport: "stdio",
				Command:   m.GetCommand(),
				Args:      m.GetArgs(),
				Headers:   m.GetEnv(),
			})
		}
		out.MCP = mcp
	}
	if h.GetSandbox() != nil {
		if err := json.Unmarshal([]byte(h.GetSandbox().GetConfig()), &out.Sandbox); err != nil {
			return nil, fmt.Errorf("sandbox: %w", err)
		}
		if out.Sandbox.Policy == "" {
			out.Sandbox.Policy = h.GetSandbox().GetBackend()
		}
	}
	if h.GetPolicy() != nil {
		out.Policy.Budget.MaxCostUSD = h.GetPolicy().GetBudgetUsd()
		if h.GetPolicy().GetGuardrails() != "" {
			if err := json.Unmarshal([]byte(h.GetPolicy().GetGuardrails()), &out.Policy.Guardrails); err != nil {
				return nil, fmt.Errorf("policy guardrails: %w", err)
			}
		}
	}
	if h.GetMemory() != nil && h.GetMemory().GetConfig() != "" {
		if err := json.Unmarshal([]byte(h.GetMemory().GetConfig()), &out.Store); err == nil {
			if out.Store == nil {
				out.Store = &StoreConfig{}
			}
		}
		if out.Store == nil {
			var tel TelemetryConfig
			if err := json.Unmarshal([]byte(h.GetMemory().GetConfig()), &tel); err == nil {
				out.Telemetry = &tel
			}
		}
	}
	sess, err := sessionFromProto(h.GetSession())
	if err != nil {
		return nil, err
	}
	out.Session = *sess
	return out, nil
}

func agentToProto(id string, a AgentSpec) *pb.Agent {
	return &pb.Agent{
		Id:          id,
		Description: a.Description,
		Model: &pb.ModelRef{
			Provider: a.Provider,
			Model:    a.Model,
		},
		Adapter: &pb.AdapterRef{
			Kind:           a.Adapter.Kind,
			TimeoutSeconds: int32(a.Adapter.TimeoutSeconds),
		},
		Prompt:    &pb.PromptRef{Id: a.SystemPrompt},
		SkillRefs: a.Skills,
		ToolRefs:  a.Tools,
	}
}

func agentFromProto(a *pb.Agent) (string, AgentSpec) {
	if a == nil {
		return "", AgentSpec{}
	}
	return a.GetId(), AgentSpec{
		ID:           a.GetId(),
		Provider:     a.GetModel().GetProvider(),
		Model:        a.GetModel().GetModel(),
		Description:  a.GetDescription(),
		Adapter:      AdapterConfig{Kind: a.GetAdapter().GetKind(), TimeoutSeconds: int(a.GetAdapter().GetTimeoutSeconds())},
		SystemPrompt: a.GetPrompt().GetId(),
		Skills:       a.GetSkillRefs(),
		Tools:        a.GetToolRefs(),
	}
}

func skillToProto(id string, s SkillSpec) *pb.Skill {
	sk := &pb.Skill{
		Id:          id,
		Description: s.Description,
	}
	if s.Prompt != "" {
		sk.Prompts = append(sk.Prompts, &pb.Prompt{Template: s.Prompt})
	}
	for _, t := range s.Tools {
		sk.Tools = append(sk.Tools, &pb.Tool{Id: t})
	}
	return sk
}

func skillFromProto(s *pb.Skill) (string, SkillSpec) {
	if s == nil {
		return "", SkillSpec{}
	}
	var prompt string
	if len(s.GetPrompts()) > 0 {
		prompt = s.GetPrompts()[0].GetTemplate()
	}
	var tools []string
	for _, t := range s.GetTools() {
		tools = append(tools, t.GetId())
	}
	return s.GetId(), SkillSpec{
		Name:        s.GetId(),
		Description: s.GetDescription(),
		Prompt:      prompt,
		Tools:       tools,
	}
}

func sessionToProto(s *SessionSpec) *pb.Session {
	if s == nil {
		return nil
	}
	out := &pb.Session{Mode: string(s.Mode)}
	switch s.Mode {
	case SingleTask, Single, MultiTurn:
		out.Single = &pb.SingleSession{
			AgentRef: s.Agent,
		}
	case Supervisor, Hierarchical:
		out.Supervisor = &pb.SupervisorSession{
			SupervisorRef: s.Agent,
		}
	case Autonomous:
		out.Autonomous = &pb.AutonomousSession{
			AgentRef: s.Agent,
		}
	case Workflow:
		if s.Workflow != nil {
			out.Workflow = &pb.WorkflowSession{
				Entry: s.Workflow.Entry,
			}
			for _, step := range s.Workflow.Steps {
				cfg, _ := toJSONString(step.Input)
				out.Workflow.Steps = append(out.Workflow.Steps, &pb.Step{
					Id:        step.ID,
					AgentRef:  step.Agent,
					PromptRef: cfg,
				})
			}
		}
	}
	return out
}

func sessionFromProto(s *pb.Session) (*SessionSpec, error) {
	if s == nil {
		return &SessionSpec{}, nil
	}
	out := &SessionSpec{Mode: HarnessSessionMode(s.GetMode())}
	switch out.Mode {
	case SingleTask, Single, MultiTurn:
		out.Agent = s.GetSingle().GetAgentRef()
	case Supervisor, Hierarchical:
		out.Agent = s.GetSupervisor().GetSupervisorRef()
	case Autonomous:
		out.Agent = s.GetAutonomous().GetAgentRef()
	case Workflow:
		wf := s.GetWorkflow()
		if wf != nil {
			out.Workflow = &WorkflowSpec{Entry: wf.GetEntry()}
			for _, step := range wf.GetSteps() {
				var input any
				if step.GetPromptRef() != "" {
					_ = json.Unmarshal([]byte(step.GetPromptRef()), &input)
				}
				out.Workflow.Steps = append(out.Workflow.Steps, StepSpec{
					ID:     step.GetId(),
					Agent:  step.GetAgentRef(),
					Input:  input,
					Output: step.GetExit(),
				})
			}
		}
	}
	return out, nil
}

func toJSONString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func stringifyMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprintf("%v", val)
	}
	return out
}

func mapFromStrings(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// IsZero reports whether no permission toggles are set.
func (p PermissionSpec) IsZero() bool {
	return !p.AllowFileWrite && !p.AllowFileDelete && !p.AllowNetwork && !p.AllowShell
}

func allowedTools(guardrails []GuardrailSpec) []string {
	var out []string
	for _, g := range guardrails {
		if g.Type == "tool_allow" {
			out = append(out, g.ID)
		}
	}
	return out
}

func deniedTools(guardrails []GuardrailSpec) []string {
	var out []string
	for _, g := range guardrails {
		if g.Type == "tool_deny" {
			out = append(out, g.ID)
		}
	}
	return out
}

func requiresApproval(guardrails []GuardrailSpec) bool {
	for _, g := range guardrails {
		if g.Action == "human_approval" {
			return true
		}
	}
	return false
}
