// Package tool defines the tool registry and built-in tools.
package tool

import (
	"context"
	"fmt"
)

// Tool is an executable capability.
type Tool interface {
	ID() string
	Description() string
	Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}

// Registry holds registered tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.ID()] = t
}

// Get returns a tool by ID.
func (r *Registry) Get(id string) (Tool, bool) {
	t, ok := r.tools[id]
	return t, ok
}

// List returns all registered tool IDs.
func (r *Registry) List() []string {
	ids := make([]string, 0, len(r.tools))
	for id := range r.tools {
		ids = append(ids, id)
	}
	return ids
}

// Built-in tools will be implemented here.

// PlaceholderTool is a placeholder for tools under development.
type PlaceholderTool struct {
	id string
}

// NewPlaceholderTool creates a placeholder tool.
func NewPlaceholderTool(id string) *PlaceholderTool {
	return &PlaceholderTool{id: id}
}

func (t *PlaceholderTool) ID() string { return t.id }

func (t *PlaceholderTool) Description() string {
	return fmt.Sprintf("placeholder tool %s", t.id)
}

func (t *PlaceholderTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "placeholder", "tool": t.id}, nil
}
