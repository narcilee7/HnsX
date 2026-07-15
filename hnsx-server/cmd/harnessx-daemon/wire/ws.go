package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/config"
)

// WSClient maintains a long-lived bidi WebSocket to the HarnessX server.
// Outbound: streamed TaskMessage / StatusUpdate / Result envelopes.
// Inbound: server-pushed Cancel / Drain / DomainInvalidation commands.
//
// P0 (W17) wires the channel up; full Cancel/Drain handling lands in W18
// alongside PauseSession / ResumeSession propagation.
type WSClient struct {
	cfg  *config.Config
	conn *websocket.Conn

	mu       sync.Mutex
	closed   bool
	dialer   *websocket.Dialer
	writeMu  sync.Mutex // serializes WriteMessage calls
}

// NewWSClient constructs a WSClient; Connect must be called before use.
func NewWSClient(cfg *config.Config) *WSClient {
	return &WSClient{
		cfg:    cfg,
		dialer: &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
	}
}

// Connect dials the server's /api/daemon/ws endpoint, authenticating with
// cfg.AuthToken when set. Returns when the handshake completes or the
// context is canceled.
func (w *WSClient) Connect(ctx context.Context) error {
	u, err := url.Parse(w.cfg.ServerURL)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = "/api/daemon/ws"

	hdr := http.Header{}
	if w.cfg.AuthToken != "" {
		hdr.Set("Authorization", "Bearer "+w.cfg.AuthToken)
	}
	conn, _, err := w.dialer.DialContext(ctx, u.String(), hdr)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()
	return nil
}

// Close releases the underlying connection.
func (w *WSClient) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

// Send enqueues one outbound message on the channel.
func (w *WSClient) Send(msg ServerEvent) error {
	w.mu.Lock()
	conn := w.conn
	closed := w.closed
	w.mu.Unlock()
	if closed || conn == nil {
		return fmt.Errorf("ws: not connected")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	envelope := Message{Type: msg.Kind, Payload: payload}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, raw)
}

// ReadLoop reads frames until ctx is canceled or the connection errors. Each
// inbound ServerEvent is delivered to onEvent. When onEvent is nil, frames
// are read and discarded (useful for tests).
func (w *WSClient) ReadLoop(ctx context.Context, onEvent func(ServerEvent)) error {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("ws: not connected")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		var p ServerEvent
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			continue
		}
		p.Kind = msg.Type
		if onEvent != nil {
			onEvent(p)
		}
	}
}

// ── Outbound envelope types ───────────────────────────────────────────────

// Message is the universal envelope for all WebSocket frames.
// Kept here (not in the multica_adapter package) because the daemon's
// wire protocol diverges slightly: the daemon emits ServerEvent-shaped
// payloads rather than the server's TaskMessage shape.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ServerEvent is the unified envelope the daemon sends to the server.
// Only the fields relevant to the chosen Kind are populated.
type ServerEvent struct {
	Kind string `json:"-"` // populated from Message.Type on receive

	// Register / Heartbeat / Deregister
	DaemonID string `json:"daemon_id,omitempty"`

	// Observation batch (one or more per envelope)
	Observations []Observation `json:"observations,omitempty"`

	// Status update
	Status *StatusUpdate `json:"status,omitempty"`

	// Final result
	Result *SessionResult `json:"result,omitempty"`
}

// Observation is one stream-json line from the agent CLI.
type Observation struct {
	SessionID   string         `json:"session_id"`
	DomainID    string         `json:"domain_id"`
	DomainVersion string       `json:"domain_version,omitempty"`
	StepID      string         `json:"step_id,omitempty"`
	AgentID     string         `json:"agent_id"`
	Kind        string         `json:"kind"`
	Payload     map[string]any `json:"payload,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAtMs int64          `json:"created_at_ms"`
}

// StatusUpdate reports a state transition on a session.
type StatusUpdate struct {
	SessionID   string `json:"session_id"`
	State       string `json:"state"`
	Message     string `json:"message,omitempty"`
	TimestampMs int64  `json:"timestamp_ms"`
}

// SessionResult is the final cost / token / output of a session.
type SessionResult struct {
	SessionID         string         `json:"session_id"`
	Result            map[string]any `json:"result,omitempty"`
	TotalCostUSD      float64        `json:"total_cost_usd"`
	TotalPromptTokens int64         `json:"total_prompt_tokens"`
	TotalCompletionTokens int64     `json:"total_completion_tokens"`
	DurationMs        int64          `json:"duration_ms"`
}
