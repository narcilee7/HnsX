package viewmodel

import "time"

// RuntimeListItem is the canonical list view of a live runtime worker.
type RuntimeListItem struct {
	RuntimeID       string            `json:"runtime_id"`
	Status          string            `json:"status"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	AgeSeconds      float64           `json:"age_seconds"`
	Healthy         bool              `json:"healthy"`
	Version         string            `json:"version,omitempty"`
	Region          string            `json:"region,omitempty"`
	Hostname        string            `json:"hostname,omitempty"`
	Pid             string            `json:"pid,omitempty"`
	Capacity        int32             `json:"capacity,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	Models          []string          `json:"models,omitempty"`
	SandboxRuntimes []string          `json:"sandbox_runtimes,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// RuntimeList is a paginated list of runtime workers.
type RuntimeList struct {
	Items  []RuntimeListItem `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// RuntimeDetail is the canonical detail view of a single runtime worker.
type RuntimeDetail struct {
	RuntimeID       string            `json:"runtime_id"`
	Status          string            `json:"status"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	AgeSeconds      float64           `json:"age_seconds"`
	Healthy         bool              `json:"healthy"`
	Version         string            `json:"version,omitempty"`
	Region          string            `json:"region,omitempty"`
	Hostname        string            `json:"hostname,omitempty"`
	Pid             string            `json:"pid,omitempty"`
	Capacity        int32             `json:"capacity,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	Models          []string          `json:"models,omitempty"`
	SandboxRuntimes []string          `json:"sandbox_runtimes,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// RuntimeInfo is the compact discovery view used by worker runtimes.
type RuntimeInfo struct {
	RuntimeID    string   `json:"runtime_id"`
	Capabilities []string `json:"capabilities"`
	Region       string   `json:"region,omitempty"`
	Version      string   `json:"version,omitempty"`
}
