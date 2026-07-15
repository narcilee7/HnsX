package multica_adapter

import "github.com/gin-gonic/gin"

// ListAutopilots handles GET /api/autopilots.
//
// P0 returns an empty list; autopilot scheduling arrives in a later phase.
func (a *Adapter) ListAutopilots(c *gin.Context) {
	writeJSON(c, 200, []any{})
}

// CreateAutopilot handles POST /api/autopilots.
func (a *Adapter) CreateAutopilot(c *gin.Context) {
	notImplemented(c, "CreateAutopilot")
}

// ListChatPinnedAgents handles GET /api/chat/pinned-agents.
//
// P0 returns an empty list; pinned-agent config lives in HnsX Settings in
// the next phase.
func (a *Adapter) ListChatPinnedAgents(c *gin.Context) {
	writeJSON(c, 200, []any{})
}
