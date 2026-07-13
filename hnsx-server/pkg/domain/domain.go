// Package spec defines the Harness-first DomainSpec v2 and its sub-types.
//
// This package is the source of truth for the shape of a Domain. It is kept
// intentionally free of infrastructure dependencies (no HTTP, no DB, no gRPC)
// so it can be imported by both the CLI and the server.
package domain

// DomainSpec is the top-level unit of business definition.
type DomainSpec struct {
	ID          string      `json:"id" yaml:"id"`
	Version     string      `json:"version" yaml:"version"`
	Description string      `json:"description" yaml:"description"`
	Harness     HarnessSpec `json:"harness" yaml:"harness"`
}

// HarnessSpec defines the constraint and execution system for autonomous agents.
type HarnessSpec struct {
	Agents        map[string]AgentSpec  `json:"agents" yaml:"agents"`
	Prompts       map[string]PromptSpec `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Skills        map[string]SkillSpec  `json:"skills,omitempty" yaml:"skills,omitempty"`
	OutputSchemas map[string]any        `json:"output_schemas,omitempty" yaml:"output_schemas,omitempty"`
	Tools         map[string]ToolConfig `json:"tools,omitempty" yaml:"tools,omitempty"`
	MCP           *MCPConfig            `json:"mcp,omitempty" yaml:"mcp,omitempty"`
	Sandbox       SandboxSpec           `json:"sandbox" yaml:"sandbox"`
	Policy        PolicySpec            `json:"policy,omitempty" yaml:"policy,omitempty"`
	Store         *StoreConfig          `json:"store,omitempty" yaml:"store,omitempty"`
	Session       SessionSpec           `json:"session" yaml:"session"`
	Telemetry     *TelemetryConfig      `json:"telemetry,omitempty" yaml:"telemetry,omitempty"`
}

// AgentSpec defines how to connect to an existing strong agent.
type AgentSpec struct {
	ID           string        `json:"id,omitempty" yaml:"id,omitempty"`
	Provider     string        `json:"provider" yaml:"provider"`
	Model        string        `json:"model,omitempty" yaml:"model,omitempty"`
	Adapter      AdapterConfig `json:"adapter" yaml:"adapter"`
	APIKeyEnv    string        `json:"api_key_env,omitempty" yaml:"api_key_env,omitempty"`
	SystemPrompt string        `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Description  string        `json:"description,omitempty" yaml:"description,omitempty"`
	SkillRefs    []string      `json:"skill_refs,omitempty" yaml:"skill_refs,omitempty"`
	ToolRefs     []string      `json:"tool_refs,omitempty" yaml:"tool_refs,omitempty"`
}

// AdapterConfig declares adapter-specific parameters. Add fields here as new
// provider integrations land (currently Anthropic, OpenAI, ClaudeCode, Ollama,
// Noop, Echo).
type AdapterConfig struct {
	Kind           string `json:"kind" yaml:"kind"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	Endpoint       string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	APIKeyEnv      string `json:"api_key_env,omitempty" yaml:"api_key_env,omitempty"`
}

// PromptSpec defines a reusable prompt unit.
type PromptSpec struct {
	Type     string `json:"type" yaml:"type"`
	Template string `json:"template" yaml:"template"`
	Schema   any    `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// SkillSpec defines a reusable business capability bundle.
type SkillSpec struct {
	Name         string        `json:"name,omitempty" yaml:"name,omitempty"`
	Description  string        `json:"description,omitempty" yaml:"description,omitempty"`
	Prompts      []PromptSpec  `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Tools        []ToolConfig  `json:"tools,omitempty" yaml:"tools,omitempty"`
	McpRefs      []string      `json:"mcp_refs,omitempty" yaml:"mcp_refs,omitempty"`
	Examples     []ExampleSpec `json:"examples,omitempty" yaml:"examples,omitempty"`
	Sandbox      *SandboxSpec  `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	OutputSchema string        `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
}

// ExampleSpec is a few-shot example for a skill.
type ExampleSpec struct {
	ID     string `json:"id,omitempty" yaml:"id,omitempty"`
	Input  string `json:"input" yaml:"input"`
	Output string `json:"output" yaml:"output"`
}

// ToolConfig defines an available tool for agents.
type ToolConfig struct {
	Kind        string `json:"kind" yaml:"kind"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Config      any    `json:"config" yaml:"config"`
}

// MCPConfig holds Model Context Protocol server definitions.
type MCPConfig struct {
	Servers []MCPServer `json:"servers" yaml:"servers"`
}

// MCPServer defines one MCP server connection.
type MCPServer struct {
	Name      string            `json:"name" yaml:"name"`
	Transport string            `json:"transport" yaml:"transport"`
	Command   string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args      []string          `json:"args,omitempty" yaml:"args,omitempty"`
	URL       string            `json:"url,omitempty" yaml:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// SandboxSpec defines execution isolation policy.
type SandboxSpec struct {
	Policy     string             `json:"policy" yaml:"policy"`
	Runtime    string             `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Filesystem *SandboxFilesystem `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Network    *SandboxNetwork    `json:"network,omitempty" yaml:"network,omitempty"`
	Commands   *SandboxCommands   `json:"commands,omitempty" yaml:"commands,omitempty"`
	Resources  *SandboxResources  `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// SandboxFilesystem constraints.
type SandboxFilesystem struct {
	Root         string   `json:"root,omitempty" yaml:"root,omitempty"`
	AllowedPaths []string `json:"allowed_paths,omitempty" yaml:"allowed_paths,omitempty"`
	ReadOnly     bool     `json:"read_only,omitempty" yaml:"read_only,omitempty"`
}

// SandboxNetwork constraints.
type SandboxNetwork struct {
	Enabled   bool     `json:"enabled" yaml:"enabled"`
	Allowlist []string `json:"allowlist,omitempty" yaml:"allowlist,omitempty"`
}

// SandboxCommands constraints.
type SandboxCommands struct {
	Allowlist []string `json:"allowlist,omitempty" yaml:"allowlist,omitempty"`
	Denylist  []string `json:"denylist,omitempty" yaml:"denylist,omitempty"`
}

// SandboxResources constraints.
type SandboxResources struct {
	MaxMemoryMB    int `json:"max_memory_mb,omitempty" yaml:"max_memory_mb,omitempty"`
	MaxCPUSeconds  int `json:"max_cpu_seconds,omitempty" yaml:"max_cpu_seconds,omitempty"`
	MaxWallSeconds int `json:"max_wall_seconds,omitempty" yaml:"max_wall_seconds,omitempty"`
}

// PolicySpec defines budget and guardrails.
type PolicySpec struct {
	Budget      BudgetSpec      `json:"budget,omitempty" yaml:"budget,omitempty"`
	Permissions PermissionSpec  `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Guardrails  []GuardrailSpec `json:"guardrails,omitempty" yaml:"guardrails,omitempty"`
	Approval    ApprovalSpec    `json:"approval,omitempty" yaml:"approval,omitempty"`
	Presets     []string        `json:"presets,omitempty" yaml:"presets,omitempty"`
}

// ApprovalSpec defines human-in-the-loop gates.
type ApprovalSpec struct {
	DefaultTimeoutSeconds int             `json:"default_timeout_seconds,omitempty" yaml:"default_timeout_seconds,omitempty"`
	RequiredFor           RequiredForSpec `json:"required_for,omitempty" yaml:"required_for,omitempty"`
}

// RequiredFor specifies which operations require approval.
type RequiredForSpec struct {
	Tools            []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	Resources        []string `json:"resources,omitempty" yaml:"resources,omitempty"`
	CostThresholdUSD float64  `json:"cost_threshold_usd,omitempty" yaml:"cost_threshold_usd,omitempty"`
}

// BudgetSpec constraints.
type BudgetSpec struct {
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" yaml:"max_cost_usd,omitempty"`
	MaxTurns   int     `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	MaxTokens  int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
}

// PermissionSpec toggles.
type PermissionSpec struct {
	AllowFileWrite  bool `json:"allow_file_write,omitempty" yaml:"allow_file_write,omitempty"`
	AllowFileDelete bool `json:"allow_file_delete,omitempty" yaml:"allow_file_delete,omitempty"`
	AllowNetwork    bool `json:"allow_network,omitempty" yaml:"allow_network,omitempty"`
	AllowShell      bool `json:"allow_shell,omitempty" yaml:"allow_shell,omitempty"`
}

// GuardrailSpec defines a runtime check.
type GuardrailSpec struct {
	ID      string `json:"id,omitempty" yaml:"id,omitempty"`
	Type    string `json:"type" yaml:"type"`
	On      string `json:"on,omitempty" yaml:"on,omitempty"`
	Action  string `json:"action,omitempty" yaml:"action,omitempty"`
	Schema  string `json:"schema,omitempty" yaml:"schema,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Config  any    `json:"config,omitempty" yaml:"config,omitempty"`
}

// StoreConfig selects per-namespace storage backends. The previous flat
// "memory" configuration is replaced by explicit context / knowledge /
// ephemeral namespaces.
type StoreConfig struct {
	Context   StoreNamespaceConfig `json:"context" yaml:"context"`
	Knowledge StoreNamespaceConfig `json:"knowledge" yaml:"knowledge"`
	Ephemeral StoreNamespaceConfig `json:"ephemeral" yaml:"ephemeral"`
}

// StoreNamespaceConfig selects the backend for one store namespace.
type StoreNamespaceConfig struct {
	Backend string `json:"backend" yaml:"backend"`
	Config  any    `json:"config,omitempty" yaml:"config,omitempty"`
}

// DefaultStoreNamespaceConfig returns the default in-memory store namespace
// configuration used when a domain omits the store block.
func DefaultStoreNamespaceConfig() StoreNamespaceConfig {
	return StoreNamespaceConfig{Backend: "in_memory"}
}

// SessionSpec defines the session/orchestration mode.
type SessionSpec struct {
	Mode          HarnessSessionMode `json:"mode" yaml:"mode"`
	Agent         string             `json:"agent,omitempty" yaml:"agent,omitempty"`
	Skill         string             `json:"skill,omitempty" yaml:"skill,omitempty"`
	TriggerSchema any                `json:"trigger_schema,omitempty" yaml:"trigger_schema,omitempty"`
	OutputSchema  string             `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	Workflow      *WorkflowSpec      `json:"workflow,omitempty" yaml:"workflow,omitempty"`
}

// WorkflowSpec is the deterministic DAG or supervisor's static fallback list.
type WorkflowSpec struct {
	Entry       string     `json:"entry" yaml:"entry"`
	Steps       []StepSpec `json:"steps" yaml:"steps"`
	Variables   any        `json:"variables,omitempty" yaml:"variables,omitempty"`
	ErrorPolicy string     `json:"error_policy,omitempty" yaml:"error_policy,omitempty"`
}

// StepSpec is a workflow step.
type StepSpec struct {
	ID        string `json:"id" yaml:"id"`
	Agent     string `json:"agent" yaml:"agent"`
	Input     any    `json:"input,omitempty" yaml:"input,omitempty"`
	Output    string `json:"output,omitempty" yaml:"output,omitempty"`
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
	Next      string `json:"next,omitempty" yaml:"next,omitempty"`
	OnError   string `json:"on_error,omitempty" yaml:"on_error,omitempty"`
}

// TelemetryConfig overrides.
type TelemetryConfig struct {
	TraceDir  string              `json:"trace_dir,omitempty" yaml:"trace_dir,omitempty"`
	Reporters []TelemetryReporter `json:"reporters,omitempty" yaml:"reporters,omitempty"`
}

// TelemetryReporter defines a telemetry sink.
type TelemetryReporter struct {
	Type string `json:"type" yaml:"type"`
	Addr string `json:"addr,omitempty" yaml:"addr,omitempty"`
}

type HarnessSessionMode string

const (
	SingleTask   HarnessSessionMode = "single-task"
	Single       HarnessSessionMode = "single"
	MultiTurn    HarnessSessionMode = "multi-turn"
	Supervisor   HarnessSessionMode = "supervisor"
	Hierarchical HarnessSessionMode = "hierarchical"
	Autonomous   HarnessSessionMode = "autonomous"
	Workflow     HarnessSessionMode = "workflow"
)
