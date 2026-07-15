package multica_adapter

import "encoding/json"

// Multica daemon WebSocket protocol envelope. Mirrors
// /Users/Zhuanz/open-source/multica/server/pkg/protocol/messages.go — the
// adapter translates between Multica TaskMessage / TaskProgress / TaskCompleted
// payloads and HnsX Observation records.

// Message is the universal envelope Multica's daemon sends over WS.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Message types (server <-> daemon, both directions).
const (
	// Server -> Daemon
	EventDaemonTaskAvailable      = "daemon.task_available"
	EventDaemonRuntimeProfilesChanged = "daemon.runtime_profiles_changed"

	// Daemon -> Server
	EventDaemonRegister  = "daemon.register"
	EventDaemonHeartbeat = "daemon.heartbeat"
	EventDaemonTaskStart = "daemon.task.start"
	EventTaskProgress    = "task.progress"
	EventTaskCompleted   = "task.completed"
	EventTaskFailed      = "task.failed"
	EventTaskMessage     = "task.message"
	EventTaskUsage       = "task.usage"
)

// DaemonRegisterPayload is sent by the daemon on first WS connection.
type DaemonRegisterPayload struct {
	DaemonID string        `json:"daemon_id"`
	AgentID  string        `json:"agent_id"`
	Runtimes []RuntimeInfo `json:"runtimes"`
}

// RuntimeInfo describes one available agent runtime on the daemon's host.
type RuntimeInfo struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// DaemonHeartbeatPayload is sent on a periodic interval (default 5s).
type DaemonHeartbeatPayload struct {
	DaemonID  string `json:"daemon_id"`
	Timestamp int64  `json:"timestamp_ms"`
}

// TaskProgressPayload is sent by the daemon during task execution.
type TaskProgressPayload struct {
	TaskID  string `json:"task_id"`
	Summary string `json:"summary"`
	Step    int    `json:"step,omitempty"`
	Total   int    `json:"total,omitempty"`
}

// TaskCompletedPayload is sent by the daemon when a task finishes.
type TaskCompletedPayload struct {
	TaskID string `json:"task_id"`
	PRURL  string `json:"pr_url,omitempty"`
	Output string `json:"output,omitempty"`
}

// TaskFailedPayload is sent by the daemon when a task fails.
type TaskFailedPayload struct {
	TaskID string `json:"task_id"`
	Error  string `json:"error,omitempty"`
}

// TaskMessagePayload is the streaming observation emitted by the daemon.
type TaskMessagePayload struct {
	TaskID    string         `json:"task_id"`
	IssueID   string         `json:"issue_id,omitempty"`
	Seq       int            `json:"seq"`
	Type      string         `json:"type"` // "text" | "tool_use" | "tool_result" | "error"
	Tool      string         `json:"tool,omitempty"`
	Content   string         `json:"content,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
}

// TaskUsagePayload reports cost / token usage for a task.
type TaskUsagePayload struct {
	TaskID            string `json:"task_id"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	DurationMs        int64  `json:"duration_ms"`
}

// TaskAvailablePayload is pushed by the server to wake a daemon up.
type TaskAvailablePayload struct {
	RuntimeID string `json:"runtime_id"`
	TaskID    string `json:"task_id,omitempty"`
}
