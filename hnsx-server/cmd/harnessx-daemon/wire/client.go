// Package wire implements the daemon ↔ server transport.
//
// P0 keeps it deliberately minimal: HTTP registration + a long-poll claim
// loop. The full WebSocket bidi channel (which streams observations and
// receives Cancel / Drain / DomainInvalidation commands) arrives in W6
// when the daemon grows the Harness engine.
package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/config"
)

// Multica's daemon REST API surface, reused by HarnessX. The endpoints map
// 1:1 onto the routes the forked Multica server exposes at /api/daemon/*.
const (
	endpointRegister  = "/api/daemon/register"
	endpointHeartbeat = "/api/daemon/heartbeat"
	endpointDeregister = "/api/daemon/deregister"
)

// Daemon is the long-running client that talks to the server over HTTP.
type Daemon struct {
	cfg  *config.Config
	http *http.Client

	mu           sync.Mutex
	assignedID   string
	runtimes     []string
	heartbeatDur time.Duration

	// OnTask is the hook invoked (in a worker goroutine) whenever ClaimTask
	// returns a task. The HarnessX engine uses this to spawn the agent
	// subprocess and stream observations back. When nil, claimed tasks are
	// logged and silently dropped — useful for the W5 skeleton + tests.
	OnTask func(ctx context.Context, task *Task) error
}

// NewDaemon constructs a Daemon bound to cfg.
func NewDaemon(cfg *config.Config) *Daemon {
	return &Daemon{
		cfg:          cfg,
		http:         &http.Client{Timeout: 60 * time.Second},
		heartbeatDur: 5 * time.Second,
	}
}

// Run blocks until ctx is canceled, performing register → heartbeat loop →
// claim loop. The caller is responsible for spawning agent subprocesses
// when ClaimTask returns a task.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.Register(ctx); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	if err := d.Heartbeat(ctx); err != nil {
		return fmt.Errorf("initial heartbeat: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.heartbeatLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		d.claimLoop(ctx)
	}()
	wg.Wait()
	_ = d.Deregister(context.Background())
	return nil
}

// Register POSTs /api/daemon/register with the configured RuntimeProfiles.
func (d *Daemon) Register(ctx context.Context) error {
	body := buildRegisterPayload(d.cfg)
	var resp registerResponse
	if err := d.doJSON(ctx, http.MethodPost, endpointRegister, body, &resp); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if resp.DaemonID != "" {
		d.assignedID = resp.DaemonID
	}
	d.runtimes = resp.Runtimes
	return nil
}

// Deregister sends a final deregister.
func (d *Daemon) Deregister(ctx context.Context) error {
	return d.doJSON(ctx, http.MethodPost, endpointDeregister,
		deregisterPayload{DaemonID: d.id()}, nil)
}

// Heartbeat sends a single heartbeat.
func (d *Daemon) Heartbeat(ctx context.Context) error {
	body := heartbeatPayload{
		DaemonID:  d.id(),
		Timestamp: time.Now().UnixMilli(),
	}
	return d.doJSON(ctx, http.MethodPost, endpointHeartbeat, body, nil)
}

// ClaimTask long-polls /tasks/claim for one runtime.
func (d *Daemon) ClaimTask(ctx context.Context, runtimeID string, maxWait time.Duration) (*Task, error) {
	body := claimRequest{
		DaemonID:       d.id(),
		MaxWaitSeconds: int(maxWait.Seconds()),
	}
	var resp claimResponse
	if err := d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/claim", runtimeID), body, &resp); err != nil {
		return nil, err
	}
	if resp.Task == nil {
		return nil, nil
	}
	return resp.Task, nil
}

// ReportProgress posts a task progress update.
func (d *Daemon) ReportProgress(ctx context.Context, runtimeID, taskID, summary string, step, total int) error {
	body := progressPayload{TaskID: taskID, Summary: summary, Step: step, Total: total}
	return d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/%s/progress", runtimeID, taskID), body, nil)
}

// ReportMessage posts a single TaskMessage.
func (d *Daemon) ReportMessage(ctx context.Context, runtimeID, taskID string, msg TaskMessage) error {
	msg.TaskID = taskID
	return d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/%s/messages", runtimeID, taskID), msg, nil)
}

// ReportComplete marks the task as completed.
func (d *Daemon) ReportComplete(ctx context.Context, runtimeID, taskID string, prURL, output string) error {
	body := completePayload{TaskID: taskID, PRURL: prURL, Output: output}
	return d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/%s/complete", runtimeID, taskID), body, nil)
}

// ReportFail marks the task as failed.
func (d *Daemon) ReportFail(ctx context.Context, runtimeID, taskID, errMsg string) error {
	body := failPayload{TaskID: taskID, Error: errMsg}
	return d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/%s/fail", runtimeID, taskID), body, nil)
}

// ReportUsage posts cost / token usage for a finished task.
func (d *Daemon) ReportUsage(ctx context.Context, runtimeID, taskID string, u Usage) error {
	u.TaskID = taskID
	return d.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/api/daemon/runtimes/%s/tasks/%s/usage", runtimeID, taskID), u, nil)
}

// ── Loops ────────────────────────────────────────────────────────────────

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(d.heartbeatDur)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := d.Heartbeat(ctx); err != nil && d.cfg.Verbose {
				fmt.Fprintf(os.Stderr, "harnessx-daemon: heartbeat: %v\n", err)
			}
		}
	}
}

func (d *Daemon) claimLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d.mu.Lock()
		runtimes := append([]string(nil), d.runtimes...)
		d.mu.Unlock()

		if len(runtimes) == 0 {
			time.Sleep(backoff)
			continue
		}

		got := false
		for _, rid := range runtimes {
			task, err := d.ClaimTask(ctx, rid, 30*time.Second)
			if err != nil {
				if d.cfg.Verbose {
					fmt.Fprintf(os.Stderr, "harnessx-daemon: claim %s: %v\n", rid, err)
				}
				continue
			}
			if task != nil {
				if d.cfg.Verbose {
					fmt.Printf("harnessx-daemon: claimed task %s on runtime %s (domain=%s)\n",
						task.ID, rid, task.AgentID)
				}
				got = true
				if d.OnTask != nil {
					go func(t *Task) { _ = d.OnTask(ctx, t) }(task)
				}
			}
		}
		if !got {
			time.Sleep(backoff)
			if backoff < 10*time.Second {
				backoff *= 2
			}
		} else {
			backoff = time.Second
		}
	}
}

// id returns the server-assigned daemon id, falling back to the configured
// id while registration is still in flight.
func (d *Daemon) id() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.assignedID != "" {
		return d.assignedID
	}
	return d.cfg.DaemonID
}

// ── Wire types ───────────────────────────────────────────────────────────

type registerPayload struct {
	DaemonID string                 `json:"daemon_id"`
	AgentID  string                 `json:"agent_id"`
	Runtimes []registerRuntimeEntry `json:"runtimes"`
}

type registerRuntimeEntry struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

func buildRegisterPayload(cfg *config.Config) registerPayload {
	out := registerPayload{
		DaemonID: cfg.DaemonID,
		AgentID:  cfg.WorkspaceID,
		Runtimes: make([]registerRuntimeEntry, 0, len(cfg.RuntimeProfiles)),
	}
	for _, p := range cfg.RuntimeProfiles {
		out.Runtimes = append(out.Runtimes, registerRuntimeEntry{
			Type:    p.Name,
			Version: "1.0",
			Status:  "online",
		})
	}
	return out
}

type registerResponse struct {
	OK        bool     `json:"ok"`
	DaemonID  string   `json:"daemon_id"`
	Runtimes  []string `json:"runtimes"`
	Heartbeat int      `json:"heartbeat_interval_seconds"`
}

type deregisterPayload struct {
	DaemonID string `json:"daemon_id"`
}

type heartbeatPayload struct {
	DaemonID  string `json:"daemon_id"`
	Timestamp int64  `json:"timestamp_ms"`
}

type claimRequest struct {
	DaemonID       string `json:"daemon_id"`
	MaxWaitSeconds int    `json:"max_wait_seconds"`
}

type claimResponse struct {
	Task *Task `json:"task"`
}

// Task mirrors the server's AgentTaskResponse.
type Task struct {
	ID          string         `json:"id"`
	AgentID     string         `json:"agent_id"`
	RuntimeID   string         `json:"runtime_id"`
	IssueID     string         `json:"issue_id"`
	WorkspaceID string         `json:"workspace_id"`
	Status      string         `json:"status"`
	Trigger     map[string]any `json:"trigger,omitempty"`
}

type progressPayload struct {
	TaskID  string `json:"task_id"`
	Summary string `json:"summary"`
	Step    int    `json:"step,omitempty"`
	Total   int    `json:"total,omitempty"`
}

// TaskMessage is one streaming observation.
type TaskMessage struct {
	TaskID    string         `json:"task_id"`
	Seq       int            `json:"seq"`
	Type      string         `json:"type"`
	Tool      string         `json:"tool,omitempty"`
	Content   string         `json:"content,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
}

type completePayload struct {
	TaskID string `json:"task_id"`
	PRURL  string `json:"pr_url,omitempty"`
	Output string `json:"output,omitempty"`
}

type failPayload struct {
	TaskID string `json:"task_id"`
	Error  string `json:"error,omitempty"`
}

// Usage reports cost / token consumption for a finished task.
type Usage struct {
	TaskID           string  `json:"task_id"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	DurationMs       int64   `json:"duration_ms"`
}

// ── Helpers ──────────────────────────────────────────────────────────────

// doJSON sends an HTTP request and decodes the JSON response into out.
func (d *Daemon) doJSON(ctx context.Context, method, path string, body any, out any) error {
	u, err := url.Parse(d.cfg.ServerURL)
	if err != nil {
		return err
	}
	u.Path = path
	endpoint := u.String()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.AuthToken)
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %d %s", method, endpoint, resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
