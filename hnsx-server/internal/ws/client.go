package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
)

// Client is the daemon-side WS client. It owns one WebSocket
// connection to the server and exposes typed methods for the
// daemon's per-tick operations (claim, write observations, status
// changes, heartbeat).
//
// Concurrency: Client is safe for concurrent use. A single read
// goroutine consumes server-pushed envelopes (currently just acks);
// each public method serializes writes via writeMu.
type Client struct {
	url    string
	dialer websocket.Dialer
	conn   *websocket.Conn

	mu     sync.Mutex // guards conn + writeMu (we only need one but keep explicit)
	idSeq  atomic.Int64

	closed chan struct{}
	once   sync.Once

	// pending holds the response channel for in-flight request ids.
	pending   map[string]chan Envelope
	pendingMu sync.Mutex
}

// NewClient constructs a Client. Call Connect to actually open the
// WebSocket.
func NewClient(serverURL string) *Client {
	return &Client{
		url: serverURL,
		dialer: websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		},
		pending: make(map[string]chan Envelope),
		closed:  make(chan struct{}),
	}
}

// Connect dials the server and starts the read loop.
func (c *Client) Connect(ctx context.Context) error {
	conn, resp, err := c.dialer.DialContext(ctx, c.url, http.Header{})
	if err != nil {
		return fmt.Errorf("ws dial: %w (status=%v)", err, resp)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go c.readLoop(conn)
	return nil
}

// Close terminates the connection. Safe to call multiple times.
func (c *Client) Close() error {
	var err error
	c.once.Do(func() {
		close(c.closed)
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			err = c.conn.Close()
		}
		c.mu.Unlock()
	})
	return err
}

// Claim asks the server for the issues currently assigned to the
// daemon's workspace. Returns the list of issues to act on.
func (c *Client) Claim(ctx context.Context, workspaceID string) ([]ClaimedIssue, error) {
	req := ClaimRequest{WorkspaceID: workspaceID, DaemonName: "hnsxd", MaxItems: 50}
	buf, _ := json.Marshal(req)
	resp, err := c.roundTrip(ctx, TypeClaim, "claim", buf)
	if err != nil {
		return nil, err
	}
	var issues IssuesResponse
	if err := json.Unmarshal(resp, &issues); err != nil {
		return nil, fmt.Errorf("ws decode issues: %w", err)
	}
	return issues.Items, nil
}

// WriteObservations sends a batch of observations to the server.
// Returns an error if the server rejects the batch.
func (c *Client) WriteObservations(ctx context.Context, batch []ObservationEvent) error {
	if len(batch) == 0 {
		return nil
	}
	buf, _ := json.Marshal(batch)
	_, err := c.roundTrip(ctx, TypeObservations, "obs", buf)
	return err
}

// UpdateStatus tells the server the daemon has moved an issue.
func (c *Client) UpdateStatus(ctx context.Context, issueID string, status issue.Status) error {
	buf, _ := json.Marshal(IssueStatusEvent{IssueID: issueID, Status: string(status)})
	_, err := c.roundTrip(ctx, TypeIssueStatus, "status", buf)
	return err
}

// Heartbeat sends a liveness ping.
func (c *Client) Heartbeat(ctx context.Context, workspaceID string) error {
	buf, _ := json.Marshal(map[string]any{"workspace_id": workspaceID})
	_, err := c.roundTrip(ctx, TypeHeartbeat, "hb", buf)
	return err
}

// roundTrip sends one envelope, waits for the matching ack/response,
// and returns the response payload (or an error).
func (c *Client) roundTrip(ctx context.Context, envType, _ string, payload []byte) ([]byte, error) {
	id := c.nextID()

	// Register the pending channel BEFORE writing so the read loop
	// can deliver the response.
	respCh := make(chan Envelope, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	env := Envelope{Type: envType, ID: id, Payload: payload}
	if err := c.writeJSON(env); err != nil {
		return nil, fmt.Errorf("ws write: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("ws client closed")
	case resp := <-respCh:
		// For TypeAck responses, the body is AckResponse with OK/Error.
		// For TypeIssues, the body is IssuesResponse (handled by caller).
		if resp.Type == TypeAck {
			var ack AckResponse
			if err := json.Unmarshal(resp.Payload, &ack); err == nil && !ack.OK {
				return nil, fmt.Errorf("ws server nack: %s", ack.Error)
			}
		}
		return resp.Payload, nil
	}
}

func (c *Client) nextID() string {
	return fmt.Sprintf("c%d", c.idSeq.Add(1))
}

func (c *Client) writeJSON(env Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("ws not connected")
	}
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(env)
}

// readLoop consumes envelopes from the server and dispatches each
// to its pending channel. Acks carry the request id so we route
// them by id; server-pushed envelopes (TypeApprovalReq in R3.5h+)
// are dispatched to a separate channel that the daemon can read.
func (c *Client) readLoop(conn *websocket.Conn) {
	defer c.Close()
	for {
		var env Envelope
		if err := conn.ReadJSON(&env); err != nil {
			return
		}
		if env.ID != "" {
			c.pendingMu.Lock()
			ch, ok := c.pending[env.ID]
			c.pendingMu.Unlock()
			if ok {
				select {
				case ch <- env:
				default:
				}
				continue
			}
		}
		// No pending request matched; this is a server-pushed envelope
		// (e.g. TypeApprovalReq). R3.5h+ adds a typed channel for these.
		_ = env
	}
}

// _ keeps the issue import live; issue.Status is part of the public
// method signature so daemon_runtime can use it.
var _ = uuid.NewString