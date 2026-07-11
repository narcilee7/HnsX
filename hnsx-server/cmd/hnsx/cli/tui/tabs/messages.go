package tabs

// SelectMsg asks a tab to select and focus a specific item by ID.
type SelectMsg struct {
	ID string
}

// FilterMsg asks a tab to apply a free-text filter.
type FilterMsg struct {
	Query string
}

// RefreshMsg asks a tab to refresh its data immediately.
type RefreshMsg struct{}

// CommandResultMsg carries the outcome of a root-level command dispatch.
type CommandResultMsg struct {
	Info  string
	Err   error
}
