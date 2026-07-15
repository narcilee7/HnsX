package multica_adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// daemonRegistry tracks daemon → runtime registrations in-memory for P0.
// A persistent daemon_registry table arrives in P1 alongside the schema bump.
type daemonRegistry struct {
	mu       sync.RWMutex
	runtimes map[string]*runtimeRecord // key: runtime_id
}

type runtimeRecord struct {
	DaemonID   string
	AgentID    string
	Type       string
	WorkspaceID string
	RegisteredAt time.Time
	LastSeen   time.Time
}

func newDaemonRegistry() *daemonRegistry {
	return &daemonRegistry{runtimes: map[string]*runtimeRecord{}}
}

func (r *daemonRegistry) register(rec *runtimeRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec.LastSeen = time.Now()
	r.runtimes[rec.AgentID] = rec
}

func (r *daemonRegistry) heartbeat(runtimeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok :=	r.runtimes[runtimeID]
	if !ok {
		return false
	}
	rec.LastSeen = time.Now()
	return true
}

func (r *daemonRegistry) list() []*runtimeRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*runtimeRecord, 0, len(r.runtimes))
	for _, rec := range r.runtimes {
		out = append(out, rec)
	}
	return out
}

// DaemonRegister handles POST /api/daemon/register.
func (a *Adapter) DaemonRegister(c *gin.Context) {
	var body DaemonRegisterPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	for _, ri := range body.Runtimes {
		a.daemons.register(&runtimeRecord{
			DaemonID:   body.DaemonID,
			AgentID:    fmt.Sprintf("%s-%s", body.DaemonID, ri.Type),
			Type:       ri.Type,
			RegisteredAt: time.Now(),
		})
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "registered": len(body.Runtimes)})
}

// DaemonHeartbeat handles POST /api/daemon/heartbeat.
func (a *Adapter) DaemonHeartbeat(c *gin.Context) {
	var body struct {
		DaemonID string `json:"daemon_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if !a.daemons.heartbeat(body.DaemonID) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "registered": false})
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// DaemonDeregister handles POST /api/daemon/deregister.
func (a *Adapter) DaemonDeregister(c *gin.Context) {
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// GetWorkspaceRepos handles GET /api/daemon/workspaces/:workspaceId/repos.
//
// P0 returns an empty list; P1 wires up real repo config from HnsX.
func (a *Adapter) GetWorkspaceRepos(c *gin.Context) {
	writeJSON(c, http.StatusOK, []any{})
}

// ClaimTask handles POST /api/daemon/runtimes/:runtimeId/tasks/claim.
//
// Multica's daemon long-polls this endpoint; the adapter pulls the oldest
// pending session from HnsX's SessionService, transitions it to running,
// and returns it shaped as a Multica AgentTaskResponse. When no work is
// available the adapter responds with {task: null} so the daemon backs off.
func (a *Adapter) ClaimTask(c *gin.Context) {
	if a.app == nil || a.app.SessionService == nil {
		writeJSON(c, http.StatusOK, gin.H{"task": nil})
		return
	}
	tid := tenantFromGin(c)
	sessions, err := a.app.SessionService.ListPending(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if len(sessions) == 0 {
		writeJSON(c, http.StatusOK, gin.H{"task": nil})
		return
	}

	// Pop the oldest pending session, transition to running, return as task.
	sess := sessions[0]
	if _, err := a.app.SessionService.MarkRunning(tid, sess.ID); err != nil {
		errorJSON(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
		return
	}
	sess.State = "running"

	runtimeID := c.Param("runtimeId")
	resp := buildAgentTaskResponse(sess, runtimeID, tid)
	writeJSON(c, http.StatusOK, gin.H{"task": resp})
}

// buildAgentTaskResponse maps an HnsX Session into Multica's AgentTaskResponse
// shape. The session's domain_id is the agent; the trigger carries the
// brief.
func buildAgentTaskResponse(s *sessionmodel.Session, runtimeID string, tid tenant.ID) AgentTaskResponse {
	title, _ := s.Trigger["issue_title"].(string)
	if title == "" {
		title = s.DomainID
	}
	desc, _ := s.Trigger["issue_description"].(string)
	brief := title
	if desc != "" {
		brief = title + "\n\n" + desc
	}

	now := s.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00")

	// Look up the agent descriptor (provider/model) so the daemon can
	// render the brief without a second roundtrip.
	var agentInfo *TaskAgentData
	if a, ok := a_lookupAgent(tid, s.DomainID); ok {
		agentInfo = a
	}

	preview := s.Trigger
	if preview == nil {
		preview = map[string]any{}
	}

	return AgentTaskResponse{
		ID:           s.ID,
		AgentID:      s.DomainID,
		RuntimeID:    runtimeID,
		IssueID:      s.ID,
		WorkspaceID:  string(tid),
		ThreadName:   s.ID,
		Status:       string(s.State),
		Priority:     0,
		DispatchedAt: &now,
		StartedAt:    &now,
		Attempt:      1,
		MaxAttempts:  1,
		Agent:        agentInfo,
		Trigger:      preview,
		BriefSummary: brief,
	}
}

// CompleteTask handles POST /api/daemon/runtimes/:runtimeId/tasks/:taskId/complete.
func (a *Adapter) CompleteTask(c *gin.Context) {
	taskID := c.Param("taskId")
	tid := a.lookupTenantForTask(taskID)
	if a.app != nil && a.app.SessionService != nil {
		if _, err := a.app.SessionService.MarkCompleted(tid, taskID, nil); err != nil {
			a.logWarn("complete task state transition failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// FailTask handles POST /api/daemon/runtimes/:runtimeId/tasks/:taskId/fail.
func (a *Adapter) FailTask(c *gin.Context) {
	taskID := c.Param("taskId")
	var body struct {
		Error string `json:"error"`
	}
	_ = c.ShouldBindJSON(&body)
	tid := a.lookupTenantForTask(taskID)
	if a.app != nil && a.app.SessionService != nil {
		if _, err := a.app.SessionService.MarkFailed(tid, taskID); err != nil {
			a.logWarn("fail task state transition failed", zap.String("task_id", taskID), zap.Error(err))
		}
		a.publishObservation(tid, taskID, domain.Observation{
			Kind:    "error",
			Payload: map[string]any{"message": body.Error, "source": "daemon_fail"},
		})
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// ReportProgress handles POST /api/daemon/runtimes/:runtimeId/tasks/:taskId/progress.
//
// Translates the Multica progress summary into an Observation (kind=progress).
func (a *Adapter) ReportProgress(c *gin.Context) {
	var body TaskProgressPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	tid := a.lookupTenantForTask(body.TaskID)
	a.publishObservation(tid, body.TaskID, domain.Observation{
		Kind:    "progress",
		Payload: map[string]any{"summary": body.Summary, "step": body.Step, "total": body.Total},
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// ReportTaskMessage handles POST /api/daemon/runtimes/:runtimeId/tasks/:taskId/messages.
//
// Translates Multica TaskMessage (text / tool_use / tool_result / error) into
// an HnsX Observation. Sequential numbers from Multica become the observation
// sequence index.
func (a *Adapter) ReportTaskMessage(c *gin.Context) {
	var body TaskMessagePayload
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	tid := a.lookupTenantForTask(body.TaskID)
	kind, payload := translateTaskMessageToObservation(body)
	a.publishObservation(tid, body.TaskID, domain.Observation{
		Kind:    kind,
		Payload: payload,
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// ReportTaskUsage handles POST /api/daemon/runtimes/:runtimeId/tasks/:taskId/usage.
func (a *Adapter) ReportTaskUsage(c *gin.Context) {
	var body TaskUsagePayload
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	tid := a.lookupTenantForTask(body.TaskID)
	a.publishObservation(tid, body.TaskID, domain.Observation{
		Kind: "cost",
		Payload: map[string]any{
			"prompt_tokens":     body.PromptTokens,
			"completion_tokens": body.CompletionTokens,
			"total_cost_usd":    body.TotalCostUSD,
			"duration_ms":       body.DurationMs,
		},
	})
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}

// DaemonWebSocket handles GET /api/daemon/ws (multica's per-daemon channel).
//
// P0 establishes the WS, registers the runtimes the daemon reports, and
// translates inbound messages into HnsX observations. Outbound notifications
// ("task_available") are not yet pushed — daemon falls back to HTTP long-poll.
func (a *Adapter) DaemonWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		a.logWarn("daemon WS upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Heartbeat ticker (server -> daemon).
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}
	}()
	defer close(done)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			a.logDebug("daemon WS read closed", zap.Error(err))
			return
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			a.logWarn("daemon WS bad envelope", zap.Error(err))
			continue
		}
		switch msg.Type {
		case EventDaemonRegister:
			var p DaemonRegisterPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				for _, ri := range p.Runtimes {
					a.daemons.register(&runtimeRecord{
						DaemonID:   p.DaemonID,
						AgentID:    fmt.Sprintf("%s-%s", p.DaemonID, ri.Type),
						Type:       ri.Type,
						RegisteredAt: time.Now(),
					})
				}
			}
		case EventDaemonHeartbeat:
			var p DaemonHeartbeatPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.daemons.heartbeat(p.DaemonID)
			}
		case EventTaskProgress:
			var p TaskProgressPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.ReportProgressBody(p)
			}
		case EventTaskCompleted:
			var p TaskCompletedPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.ReportCompletedBody(p)
			}
		case EventTaskFailed:
			var p TaskFailedPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.ReportFailedBody(p)
			}
		case EventTaskMessage:
			var p TaskMessagePayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.ReportTaskMessageBody(p)
			}
		case EventTaskUsage:
			var p TaskUsagePayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				a.ReportTaskUsageBody(p)
			}
		default:
			a.logDebug("daemon WS unknown message type", zap.String("type", msg.Type))
		}
	}
}

// Body-shaped reporting helpers used by both HTTP and WS paths.
func (a *Adapter) ReportProgressBody(p TaskProgressPayload) {
	tid := a.lookupTenantForTask(p.TaskID)
	a.publishObservation(tid, p.TaskID, domain.Observation{
		Kind:    "progress",
		Payload: map[string]any{"summary": p.Summary, "step": p.Step, "total": p.Total},
	})
}

func (a *Adapter) ReportCompletedBody(p TaskCompletedPayload) {
	tid := a.lookupTenantForTask(p.TaskID)
	if a.app != nil && a.app.SessionService != nil {
		if _, err := a.app.SessionService.MarkCompleted(tid, p.TaskID, nil); err != nil {
			a.logWarn("ws complete state transition failed", zap.Error(err))
		}
	}
}

func (a *Adapter) ReportFailedBody(p TaskFailedPayload) {
	tid := a.lookupTenantForTask(p.TaskID)
	if a.app != nil && a.app.SessionService != nil {
		if _, err := a.app.SessionService.MarkFailed(tid, p.TaskID); err != nil {
			a.logWarn("ws fail state transition failed", zap.Error(err))
		}
	}
	a.publishObservation(tid, p.TaskID, domain.Observation{
		Kind:    "error",
		Payload: map[string]any{"message": p.Error},
	})
}

func (a *Adapter) ReportTaskMessageBody(p TaskMessagePayload) {
	tid := a.lookupTenantForTask(p.TaskID)
	kind, payload := translateTaskMessageToObservation(p)
	a.publishObservation(tid, p.TaskID, domain.Observation{
		Kind:    kind,
		Payload: payload,
	})
}

func (a *Adapter) ReportTaskUsageBody(p TaskUsagePayload) {
	tid := a.lookupTenantForTask(p.TaskID)
	a.publishObservation(tid, p.TaskID, domain.Observation{
		Kind: "cost",
		Payload: map[string]any{
			"prompt_tokens":     p.PromptTokens,
			"completion_tokens": p.CompletionTokens,
			"total_cost_usd":    p.TotalCostUSD,
			"duration_ms":       p.DurationMs,
		},
	})
}

// translateTaskMessageToObservation maps Multica's text / tool_use /
// tool_result / error kinds onto HnsX's observation vocabulary.
func translateTaskMessageToObservation(m TaskMessagePayload) (string, map[string]any) {
	switch m.Type {
	case "text":
		return "text", map[string]any{"content": m.Content}
	case "tool_use":
		return "tool_call", map[string]any{
			"tool":  m.Tool,
			"input": m.Input,
		}
	case "tool_result":
		return "tool_result", map[string]any{
			"tool":   m.Tool,
			"output": m.Output,
		}
	case "error":
		return "error", map[string]any{"message": m.Content}
	}
	return m.Type, map[string]any{"content": m.Content}
}

// publishObservation forwards the observation through the api.Server
// broadcaster + TraceService when available.
func (a *Adapter) publishObservation(tid tenant.ID, sessionID string, obs domain.Observation) {
	obs.SessionID = sessionID
	if a.api != nil {
		a.api.PublishObservation(sessionID, obs)
		if a.api.TraceService != nil {
			_ = a.api.TraceService.Record(context.Background(), obs)
		}
	}
}

// emitObs is a convenience wrapper for callers that don't want to import
// domain types just to publish one observation. The kind + payload are
// sufficient to populate the Observation record.
func (a *Adapter) emitObs(tid tenant.ID, sessionID, kind string, payload map[string]any) {
	a.publishObservation(tid, sessionID, domain.Observation{
		Kind:    kind,
		Payload: payload,
	})
}

// lookupTenantForTask resolves the tenant that owns a session. P0 always
// falls back to the default tenant; P1 will index sessions by daemon_id for
// proper tenant routing.
func (a *Adapter) lookupTenantForTask(taskID string) tenant.ID {
	_ = taskID
	return tenant.DefaultID
}

// a_lookupAgent is a package-private helper that resolves an HnsX Domain
// into the Agent descriptor embedded in a task brief. Returns (nil, false)
// when no app or no DomainService is wired in.
func a_lookupAgent(tid tenant.ID, domainID string) (*TaskAgentData, bool) {
	return nil, false
}

// logWarn / logDebug funnel through the application logger when present.
func (a *Adapter) logWarn(msg string, fields ...zap.Field) {
	if a.api != nil && a.api.Logger != nil {
		a.api.Logger.Warn(msg, fields...)
	}
}

func (a *Adapter) logDebug(msg string, fields ...zap.Field) {
	if a.api != nil && a.api.Logger != nil {
		a.api.Logger.Debug(msg, fields...)
	}
}
