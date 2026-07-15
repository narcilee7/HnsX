package multica_adapter

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SquadRouteRequest is the payload a squad leader agent posts when it has
// decided which specialist to delegate to. Multica's squad_briefing.go
// calls a similar endpoint upstream; this adapter routes through the
// HarnessX WorkflowSession runner instead of relying on prompt-only
// routing.
type SquadRouteRequest struct {
	SquadID    string   `json:"squad_id"`
	IssueID    string   `json:"issue_id"`
	Outcome    string   `json:"outcome"` // "action" | "no_action" | "failed"
	Reason     string   `json:"reason"`
	Mentioned  []string `json:"mentioned"` // agent IDs the leader @-mentioned
	DomainSpec map[string]any `json:"domain_spec,omitempty"` // optional override
}

// SquadRouteResponse reports the routing decision after HarnessX transitions
// override the LLM's pick.
type SquadRouteResponse struct {
	Decision     string   `json:"decision"`     // "delegate" | "no_action" | "deny"
	Override     bool     `json:"override"`     // true when HarnessX overrode the LLM
	TargetAgent  string   `json:"target_agent,omitempty"`
	Reason       string   `json:"reason"`
	Observations []string `json:"observations,omitempty"` // routing_decision observations
}

// SquadRoutingDecision implements the W15 hook: a Squad leader's delegation
// is checked against HnsX WorkflowSession transitions before being executed.
// When the Domain has explicit transitions, they win; otherwise we fall
// back to the leader's @-mention pick.
//
// Multica's existing /api/issues/:id/squad-evaluated endpoint only records
// the leader's outcome (no override). This handler exposes a higher-level
// decision API that next.js can call to render a deterministic routing UI.
func (a *Adapter) SquadRoutingDecision(c *gin.Context) {
	var body SquadRouteRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.SquadID == "" || body.IssueID == "" {
		errorJSON(c, http.StatusBadRequest, "MISSING_IDS", "squad_id and issue_id are required")
		return
	}

	resp := SquadRouteResponse{Decision: "delegate"}

	// W15 reference implementation: if the leader mentioned no specialist,
	// fall back to no_action. If multiple were mentioned, pick the first.
	if len(body.Mentioned) == 0 {
		resp.Decision = "no_action"
		resp.Reason = "no specialist @-mentioned"
		a.emitRoutingObservation(body, resp)
		writeJSON(c, http.StatusOK, resp)
		return
	}
	resp.TargetAgent = body.Mentioned[0]
	resp.Reason = "first-mentioned"

	// If the DomainSpec carries transitions, prefer those over the LLM pick.
	if overrides := extractTransitionOverrides(body.DomainSpec, body.Mentioned); len(overrides) > 0 {
		resp.TargetAgent = overrides[0]
		resp.Override = true
		resp.Reason = "HarnessX transition override"
	}

	a.emitRoutingObservation(body, resp)
	writeJSON(c, http.StatusOK, resp)
}

// emitRoutingObservation writes a routing_decision Observation so the SSE
// stream (and Eval pipeline in P1+) sees how the leader decided.
func (a *Adapter) emitRoutingObservation(req SquadRouteRequest, resp SquadRouteResponse) {
	tid := a.lookupTenantForTask(req.IssueID)
	payload := map[string]any{
		"squad_id":     req.SquadID,
		"decision":     resp.Decision,
		"target_agent": resp.TargetAgent,
		"override":     resp.Override,
		"reason":       resp.Reason,
		"mentioned":    req.Mentioned,
	}
	a.emitObs(tid, req.IssueID, "routing_decision", payload)
}

// extractTransitionOverrides reads transition rules from a DomainSpec map.
// A transition looks like:
//
//	{"from": "classify", "when": "answer == billing", "to": "billing-agent"}
//
// P0 only inspects the static structure; later phases wire this into the
// WorkflowSession runner so transitions drive the actual agent dispatch.
func extractTransitionOverrides(spec map[string]any, mentioned []string) []string {
	if spec == nil {
		return nil
	}
	harness, _ := spec["harness"].(map[string]any)
	if harness == nil {
		return nil
	}
	session, _ := harness["session"].(map[string]any)
	if session == nil {
		return nil
	}
	workflow, _ := session["workflow"].(map[string]any)
	if workflow == nil {
		return nil
	}
	steps, _ := workflow["steps"].([]any)
	if steps == nil {
		return nil
	}

	overrides := []string{}
	for _, step := range steps {
		m, _ := step.(map[string]any)
		agent, _ := m["agent"].(string)
		if agent == "" {
			continue
		}
		for _, m := range mentioned {
			if strings.EqualFold(agent, m) {
				overrides = append(overrides, agent)
			}
		}
	}
	return overrides
}

// ObservationPayload is a small wrapper around an observation published by
// the adapter. The actual shape is owned by hnsx_server/pkg/domain.
type ObservationPayload struct {
	Kind    string
	Payload []byte
}
