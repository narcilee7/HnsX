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
		schema, err := toJSONString(p.Schema)
		if err != nil {
			return nil, fmt.Errorf("prompt %q schema: %w", id, err)
		}
		out.Prompts = append(out.Prompts, &pb.Prompt{
			Id:          id,
			Template:    p.Template,
			PromptType:  p.Type,
			Schema:      schema,
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
			Name:        t.Name,
			Description: t.Description,
			Kind:        t.Kind,
			Config:      cfg,
		})
	}
	if h.MCP != nil {
		for _, m := range h.MCP.Servers {
			out.Mcps = append(out.Mcps, &pb.MCPConfig{
				Id:        m.Name,
				Transport: m.Transport,
				Command:   m.Command,
				Args:      m.Args,
				Url:       m.URL,
				Headers:   m.Headers,
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
	policy, err := policyToProto(&h.Policy)
	if err != nil {
		return nil, err
	}
	out.Policy = policy
	if h.Store != nil {
		out.Store, err = storeToProto(h.Store)
		if err != nil {
			return nil, err
		}
	}
	if h.Telemetry != nil {
		cfg, err := toJSONString(h.Telemetry)
		if err != nil {
			return nil, fmt.Errorf("telemetry config: %w", err)
		}
		if out.Store == nil {
			out.Store = &pb.Store{}
		}
		// Preserve telemetry in context namespace config as a fallback.
		out.Store.Context = &pb.StoreNamespaceConfig{Backend: "telemetry", Config: cfg}
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
		var schema any
		if p.GetSchema() != "" {
			_ = json.Unmarshal([]byte(p.GetSchema()), &schema)
		}
		out.Prompts[p.GetId()] = PromptSpec{
			Type:     p.GetPromptType(),
			Template: p.GetTemplate(),
			Schema:   schema,
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
			Kind:        t.GetKind(),
			Name:        t.GetName(),
			Description: t.GetDescription(),
			Config:      cfg,
		}
	}
	if len(h.GetMcps()) > 0 {
		mcp := &MCPConfig{}
		for _, m := range h.GetMcps() {
			mcp.Servers = append(mcp.Servers, MCPServer{
				Name:      m.GetId(),
				Transport: m.GetTransport(),
				Command:   m.GetCommand(),
				Args:      m.GetArgs(),
				URL:       m.GetUrl(),
				Headers:   m.GetHeaders(),
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
		policy, err := policyFromProto(h.GetPolicy())
		if err != nil {
			return nil, err
		}
		out.Policy = *policy
	}
	if h.GetStore() != nil {
		store, err := storeFromProto(h.GetStore())
		if err != nil {
			return nil, err
		}
		out.Store = store
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
		Id:           id,
		Description:  a.Description,
		Model: &pb.ModelRef{
			Provider: a.Provider,
			Model:    a.Model,
		},
		Adapter: &pb.AdapterRef{
			Kind:           a.Adapter.Kind,
			TimeoutSeconds: int32(a.Adapter.TimeoutSeconds),
			Endpoint:       a.Adapter.Endpoint,
			ApiKeyEnv:      a.Adapter.APIKeyEnv,
		},
		SystemPrompt: a.SystemPrompt,
		SkillRefs:    a.SkillRefs,
		ToolRefs:     a.ToolRefs,
		ApiKeyEnv:    a.APIKeyEnv,
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
		Adapter: AdapterConfig{
			Kind:           a.GetAdapter().GetKind(),
			TimeoutSeconds: int(a.GetAdapter().GetTimeoutSeconds()),
			Endpoint:       a.GetAdapter().GetEndpoint(),
			APIKeyEnv:      a.GetAdapter().GetApiKeyEnv(),
		},
		SystemPrompt: a.GetSystemPrompt(),
		APIKeyEnv:    a.GetApiKeyEnv(),
		SkillRefs:    a.GetSkillRefs(),
		ToolRefs:     a.GetToolRefs(),
	}
}

func skillToProto(id string, s SkillSpec) *pb.Skill {
	sk := &pb.Skill{
		Id:          id,
		Description: s.Description,
	}
	for _, p := range s.Prompts {
		schema, _ := toJSONString(p.Schema)
		sk.Prompts = append(sk.Prompts, &pb.Prompt{
			Id:         p.Type,
			Template:   p.Template,
			PromptType: p.Type,
			Schema:     schema,
		})
	}
	for _, t := range s.Tools {
		cfg, _ := toJSONString(t.Config)
		sk.Tools = append(sk.Tools, &pb.Tool{
			Id:          t.Name,
			Name:        t.Name,
			Description: t.Description,
			Kind:        t.Kind,
			Config:      cfg,
		})
	}
	for _, ref := range s.McpRefs {
		sk.McpRefs = append(sk.McpRefs, ref)
	}
	for _, ex := range s.Examples {
		sk.Examples = append(sk.Examples, &pb.Example{
			Id:     ex.ID,
			Input:  ex.Input,
			Output: ex.Output,
		})
	}
	return sk
}

func skillFromProto(s *pb.Skill) (string, SkillSpec) {
	if s == nil {
		return "", SkillSpec{}
	}
	var prompts []PromptSpec
	for _, p := range s.GetPrompts() {
		var schema any
		if p.GetSchema() != "" {
			_ = json.Unmarshal([]byte(p.GetSchema()), &schema)
		}
		prompts = append(prompts, PromptSpec{
			Type:     p.GetPromptType(),
			Template: p.GetTemplate(),
			Schema:   schema,
		})
	}
	var tools []ToolConfig
	for _, t := range s.GetTools() {
		var cfg any
		if t.GetConfig() != "" {
			_ = json.Unmarshal([]byte(t.GetConfig()), &cfg)
		}
		tools = append(tools, ToolConfig{
			Kind:        t.GetKind(),
			Name:        t.GetName(),
			Description: t.GetDescription(),
			Config:      cfg,
		})
	}
	var examples []ExampleSpec
	for _, ex := range s.GetExamples() {
		examples = append(examples, ExampleSpec{
			ID:     ex.GetId(),
			Input:  ex.GetInput(),
			Output: ex.GetOutput(),
		})
	}
	return s.GetId(), SkillSpec{
		Name:        s.GetId(),
		Description: s.GetDescription(),
		Prompts:     prompts,
		Tools:       tools,
		McpRefs:     s.GetMcpRefs(),
		Examples:    examples,
	}
}

func sessionToProto(s *SessionSpec) *pb.Session {
	if s == nil {
		return nil
	}
	triggerSchema, _ := toJSONString(s.TriggerSchema)
	out := &pb.Session{
		Mode:          string(s.Mode),
		TriggerSchema: triggerSchema,
		OutputSchema:  s.OutputSchema,
	}
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
				input, _ := toJSONString(step.Input)
				out.Workflow.Steps = append(out.Workflow.Steps, &pb.Step{
					Id:          step.ID,
					AgentRef:    step.Agent,
					Input:       input,
					Output:      step.Output,
					Condition:   step.Condition,
					Next:        step.Next,
					OnError:     step.OnError,
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
	if s.GetTriggerSchema() != "" {
		_ = json.Unmarshal([]byte(s.GetTriggerSchema()), &out.TriggerSchema)
	}
	out.OutputSchema = s.GetOutputSchema()
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
				if step.GetInput() != "" {
					_ = json.Unmarshal([]byte(step.GetInput()), &input)
				}
				out.Workflow.Steps = append(out.Workflow.Steps, StepSpec{
					ID:        step.GetId(),
					Agent:     step.GetAgentRef(),
					Input:     input,
					Output:    step.GetOutput(),
					Condition: step.GetCondition(),
					Next:      step.GetNext(),
					OnError:   step.GetOnError(),
				})
			}
		}
	}
	return out, nil
}

func policyToProto(p *PolicySpec) (*pb.Policy, error) {
	if p == nil {
		return nil, nil
	}
	return &pb.Policy{
		Budget: &pb.Budget{
			MaxCostUsd: p.Budget.MaxCostUSD,
			MaxTurns:   int32(p.Budget.MaxTurns),
			MaxTokens:  int32(p.Budget.MaxTokens),
		},
		Permissions: &pb.Permission{
			AllowFileWrite:  p.Permissions.AllowFileWrite,
			AllowFileDelete: p.Permissions.AllowFileDelete,
			AllowNetwork:    p.Permissions.AllowNetwork,
			AllowShell:      p.Permissions.AllowShell,
		},
		Guardrails: guardrailSpecsToProto(p.Guardrails),
		Approval: &pb.Approval{
			DefaultTimeoutSeconds: int32(p.Approval.DefaultTimeoutSeconds),
			RequiredFor: &pb.RequiredFor{
				Tools:            p.Approval.RequiredFor.Tools,
				Resources:        p.Approval.RequiredFor.Resources,
				CostThresholdUsd: p.Approval.RequiredFor.CostThresholdUSD,
			},
		},
		Presets: p.Presets,
	}, nil
}

func guardrailSpecsToProto(guardrails []GuardrailSpec) []*pb.Guardrail {
	out := make([]*pb.Guardrail, 0, len(guardrails))
	for _, g := range guardrails {
		cfg, _ := toJSONString(g.Config)
		out = append(out, &pb.Guardrail{
			Id:      g.ID,
			Type:    g.Type,
			On:      g.On,
			Action:  g.Action,
			Schema:  g.Schema,
			Message: g.Message,
			Config:  cfg,
		})
	}
	return out
}

func policyFromProto(p *pb.Policy) (*PolicySpec, error) {
	if p == nil {
		return &PolicySpec{}, nil
	}
	out := &PolicySpec{
		Budget: BudgetSpec{
			MaxCostUSD: p.GetBudget().GetMaxCostUsd(),
			MaxTurns:   int(p.GetBudget().GetMaxTurns()),
			MaxTokens:  int(p.GetBudget().GetMaxTokens()),
		},
		Permissions: PermissionSpec{
			AllowFileWrite:  p.GetPermissions().GetAllowFileWrite(),
			AllowFileDelete: p.GetPermissions().GetAllowFileDelete(),
			AllowNetwork:    p.GetPermissions().GetAllowNetwork(),
			AllowShell:      p.GetPermissions().GetAllowShell(),
		},
		Approval: ApprovalSpec{
			DefaultTimeoutSeconds: int(p.GetApproval().GetDefaultTimeoutSeconds()),
			RequiredFor: RequiredForSpec{
				Tools:            p.GetApproval().GetRequiredFor().GetTools(),
				Resources:        p.GetApproval().GetRequiredFor().GetResources(),
				CostThresholdUSD: p.GetApproval().GetRequiredFor().GetCostThresholdUsd(),
			},
		},
		Presets: p.GetPresets(),
	}
	for _, g := range p.GetGuardrails() {
		var cfg any
		if g.GetConfig() != "" {
			_ = json.Unmarshal([]byte(g.GetConfig()), &cfg)
		}
		out.Guardrails = append(out.Guardrails, GuardrailSpec{
			ID:      g.GetId(),
			Type:    g.GetType(),
			On:      g.GetOn(),
			Action:  g.GetAction(),
			Schema:  g.GetSchema(),
			Message: g.GetMessage(),
			Config:  cfg,
		})
	}
	return out, nil
}

func storeToProto(s *StoreConfig) (*pb.Store, error) {
	if s == nil {
		return nil, nil
	}
	ctxCfg, _ := toJSONString(s.Context.Config)
	knowCfg, _ := toJSONString(s.Knowledge.Config)
	ephCfg, _ := toJSONString(s.Ephemeral.Config)
	return &pb.Store{
		Context:   &pb.StoreNamespaceConfig{Backend: s.Context.Backend, Config: ctxCfg},
		Knowledge: &pb.StoreNamespaceConfig{Backend: s.Knowledge.Backend, Config: knowCfg},
		Ephemeral: &pb.StoreNamespaceConfig{Backend: s.Ephemeral.Backend, Config: ephCfg},
	}, nil
}

func storeFromProto(s *pb.Store) (*StoreConfig, error) {
	if s == nil {
		return nil, nil
	}
	out := &StoreConfig{}
	if s.GetContext() != nil {
		out.Context.Backend = s.GetContext().GetBackend()
		if s.GetContext().GetConfig() != "" {
			_ = json.Unmarshal([]byte(s.GetContext().GetConfig()), &out.Context.Config)
		}
	}
	if s.GetKnowledge() != nil {
		out.Knowledge.Backend = s.GetKnowledge().GetBackend()
		if s.GetKnowledge().GetConfig() != "" {
			_ = json.Unmarshal([]byte(s.GetKnowledge().GetConfig()), &out.Knowledge.Config)
		}
	}
	if s.GetEphemeral() != nil {
		out.Ephemeral.Backend = s.GetEphemeral().GetBackend()
		if s.GetEphemeral().GetConfig() != "" {
			_ = json.Unmarshal([]byte(s.GetEphemeral().GetConfig()), &out.Ephemeral.Config)
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

// IsZero reports whether no permission toggles are set.
func (p PermissionSpec) IsZero() bool {
	return !p.AllowFileWrite && !p.AllowFileDelete && !p.AllowNetwork && !p.AllowShell
}
