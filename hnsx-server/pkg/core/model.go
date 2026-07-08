// Package core defines the DomainSpec model and loader.
package core

// DomainSpec is the top-level declarative harness definition.
type DomainSpec struct {
	ID          string  `yaml:"id" json:"id"`
	Version     string  `yaml:"version" json:"version"`
	Description string  `yaml:"description" json:"description"`
	Harness     Harness `yaml:"harness" json:"harness"`
}

// Harness is the set of constraints and capabilities wrapped around an Agent.
type Harness struct {
	Agents  []Agent     `yaml:"agents" json:"agents"`
	Prompts []Prompt    `yaml:"prompts,omitempty" json:"prompts,omitempty"`
	Skills  []Skill     `yaml:"skills,omitempty" json:"skills,omitempty"`
	Tools   []Tool      `yaml:"tools,omitempty" json:"tools,omitempty"`
	MCPs    []MCPConfig `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	Sandbox Sandbox     `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	Policy  Policy      `yaml:"policy,omitempty" json:"policy,omitempty"`
	Memory  Memory      `yaml:"memory,omitempty" json:"memory,omitempty"`
	Session Session     `yaml:"session" json:"session"`
	Eval    Eval        `yaml:"eval,omitempty" json:"eval,omitempty"`
}

// Agent describes an external agent to be harnessed.
type Agent struct {
	ID        string    `yaml:"id" json:"id"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
	Model     ModelRef  `yaml:"model" json:"model"`
	Adapter   AdapterRef `yaml:"adapter" json:"adapter"`
	Prompt    PromptRef `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	SkillRefs []string  `yaml:"skill_refs,omitempty" json:"skill_refs,omitempty"`
	ToolRefs  []string  `yaml:"tool_refs,omitempty" json:"tool_refs,omitempty"`
}

// ModelRef references a model provider and model name.
type ModelRef struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
}

// AdapterRef references an adapter configuration.
type AdapterRef struct {
	Kind           string `yaml:"kind" json:"kind"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}

// Prompt is a reusable prompt template.
type Prompt struct {
	ID        string            `yaml:"id" json:"id"`
	Template  string            `yaml:"template" json:"template"`
	Variables map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// PromptRef references a prompt by ID.
type PromptRef struct {
	ID string `yaml:"id" json:"id"`
}

// Skill is a reusable business capability package.
type Skill struct {
	ID          string   `yaml:"id" json:"id"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Prompts     []Prompt `yaml:"prompts,omitempty" json:"prompts,omitempty"`
	Tools       []Tool   `yaml:"tools,omitempty" json:"tools,omitempty"`
	MCPRefs     []string `yaml:"mcp_refs,omitempty" json:"mcp_refs,omitempty"`
}

// Tool is an atomic capability.
type Tool struct {
	ID          string                 `yaml:"id" json:"id"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string                 `yaml:"type" json:"type"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// MCPConfig describes an MCP server connection.
type MCPConfig struct {
	ID   string            `yaml:"id" json:"id"`
	Command string         `yaml:"command" json:"command"`
	Args []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env  map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// Sandbox defines execution isolation.
type Sandbox struct {
	Backend string                 `yaml:"backend" json:"backend"`
	Config  map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// Policy defines constraints.
type Policy struct {
	BudgetUSD            float64                `yaml:"budget_usd,omitempty" json:"budget_usd,omitempty"`
	AllowedTools         []string               `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	DeniedTools          []string               `yaml:"denied_tools,omitempty" json:"denied_tools,omitempty"`
	RequireHumanApproval bool                   `yaml:"require_human_approval,omitempty" json:"require_human_approval,omitempty"`
	Guardrails           map[string]interface{} `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
}

// Memory defines context storage.
type Memory struct {
	Backend string                 `yaml:"backend" json:"backend"`
	Config  map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// Session defines session execution mode.
type Session struct {
	Mode     string    `yaml:"mode" json:"mode"`
	Workflow *Workflow `yaml:"workflow,omitempty" json:"workflow,omitempty"`
}

// Workflow defines a DAG of steps for workflow mode.
type Workflow struct {
	Entry string `yaml:"entry" json:"entry"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// Step is a single node in a workflow.
type Step struct {
	ID         string            `yaml:"id" json:"id"`
	AgentRef   string            `yaml:"agent" json:"agent"`
	PromptRef  string            `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Output     string            `yaml:"output,omitempty" json:"output,omitempty"`
	Input      map[string]string `yaml:"input,omitempty" json:"input,omitempty"`
	Next       []string          `yaml:"next,omitempty" json:"next,omitempty"`
}

// Eval defines evaluation sets.
type Eval struct {
	Sets []EvalSet `yaml:"sets,omitempty" json:"sets,omitempty"`
}

// EvalSet is a collection of eval cases.
type EvalSet struct {
	ID          string     `yaml:"id" json:"id"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Cases       []EvalCase `yaml:"cases" json:"cases"`
}

// EvalCase is a single evaluation case.
type EvalCase struct {
	ID     string                 `yaml:"id" json:"id"`
	Name   string                 `yaml:"name" json:"name"`
	Input  map[string]interface{} `yaml:"input" json:"input"`
	Expect map[string]interface{} `yaml:"expect" json:"expect"`
	Scorer string                 `yaml:"scorer" json:"scorer"`
}
