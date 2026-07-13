package viewmodel

// Metrics is the canonical aggregate metrics view returned by GET /api/v1/metrics.
type Metrics struct {
	DomainID          string  `json:"domain_id"`
	TotalSessions     int     `json:"total_sessions"`
	CompletedSessions int     `json:"completed_sessions"`
	FailedSessions    int     `json:"failed_sessions"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	AgentInvocations  int     `json:"agent_invocations"`
	ToolInvocations   int     `json:"tool_invocations"`
	PromptTokens      int     `json:"prompt_tokens"`
	CompletionTokens  int     `json:"completion_tokens"`
}
