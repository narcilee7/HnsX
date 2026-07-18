package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ServerUpgrader returns the gorilla/websocket upgrader used by the
// /ws/daemon gin handler. Configured to be lenient (no Origin check)
// since hnsxd's WS endpoint is intended for daemon-to-server traffic
// on a trusted network, not browser clients.
var ServerUpgrader = websocket.Upgrader{
	ReadBufferSize:  4 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ServerConn is one side of a /ws/daemon connection. The server-side
// reads Envelopes from the daemon, applies them via the supplied
// Handler, and writes responses. A ServerConn owns one goroutine
// for reads; the caller is expected to call Write to push events.
type ServerConn struct {
	ws      *websocket.Conn
	logger  *slog.Logger
	handler Handler
	writeMu sync.Mutex
}

// Handler applies a received envelope and returns the response
// envelope (or nil to send no response). Implemented by app/.
type Handler interface {
	HandleClaim(ctx context.Context, req ClaimRequest) (IssuesResponse, error)
	HandleObservations(ctx context.Context, obs []ObservationEvent) error
	HandleIssueStatus(ctx context.Context, evt IssueStatusEvent) error
	HandleApprovalReply(ctx context.Context, rep ApprovalReplyEvent) error
	HandleHeartbeat(ctx context.Context, workspaceID string) error
}

// NewServerConn wraps an already-upgraded gorilla/websocket.Conn.
// Starts the read loop in a background goroutine; returns immediately.
func NewServerConn(ws *websocket.Conn, handler Handler, logger *slog.Logger) *ServerConn {
	if logger == nil {
		logger = slog.Default()
	}
	c := &ServerConn{ws: ws, logger: logger, handler: handler}
	go c.readLoop()
	return c
}

// Write sends one envelope to the daemon. Safe for concurrent use
// (the write goroutine + read loop can both call Write).
func (c *ServerConn) Write(env Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.ws.WriteJSON(env)
}

// Close terminates the connection.
func (c *ServerConn) Close() error {
	return c.ws.Close()
}

// readLoop consumes envelopes from the daemon and dispatches to the
// Handler. Errors are logged; the connection is closed on protocol
// errors.
func (c *ServerConn) readLoop() {
	defer c.Close()
	for {
		var env Envelope
		if err := c.ws.ReadJSON(&env); err != nil {
			c.logger.Info("ws: daemon disconnected", "err", err)
			return
		}
		c.dispatch(env)
	}
}

func (c *ServerConn) dispatch(env Envelope) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var resp Envelope
	switch env.Type {
	case TypeClaim:
		var req ClaimRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			c.ack(env.ID, false, "bad payload: "+err.Error())
			return
		}
		items, err := c.handler.HandleClaim(ctx, req)
		if err != nil {
			c.ack(env.ID, false, err.Error())
			return
		}
		resp.Type = TypeIssues
		resp.ID = env.ID
		buf, _ := json.Marshal(items)
		resp.Payload = buf
		_ = c.Write(resp)
	case TypeObservations:
		var batch []ObservationEvent
		if err := json.Unmarshal(env.Payload, &batch); err != nil {
			c.ack(env.ID, false, "bad payload: "+err.Error())
			return
		}
		if err := c.handler.HandleObservations(ctx, batch); err != nil {
			c.ack(env.ID, false, err.Error())
			return
		}
		c.ack(env.ID, true, "")
	case TypeIssueStatus:
		var evt IssueStatusEvent
		if err := json.Unmarshal(env.Payload, &evt); err != nil {
			c.ack(env.ID, false, "bad payload: "+err.Error())
			return
		}
		if err := c.handler.HandleIssueStatus(ctx, evt); err != nil {
			c.ack(env.ID, false, err.Error())
			return
		}
		c.ack(env.ID, true, "")
	case TypeApprovalReply:
		var rep ApprovalReplyEvent
		if err := json.Unmarshal(env.Payload, &rep); err != nil {
			c.ack(env.ID, false, "bad payload: "+err.Error())
			return
		}
		if err := c.handler.HandleApprovalReply(ctx, rep); err != nil {
			c.ack(env.ID, false, err.Error())
			return
		}
		c.ack(env.ID, true, "")
	case TypeHeartbeat:
		var p struct {
			WorkspaceID string `json:"workspace_id"`
		}
		_ = json.Unmarshal(env.Payload, &p)
		if err := c.handler.HandleHeartbeat(ctx, p.WorkspaceID); err != nil {
			c.ack(env.ID, false, err.Error())
			return
		}
		c.ack(env.ID, true, "")
	default:
		c.ack(env.ID, false, "unknown envelope type: "+env.Type)
	}
}

func (c *ServerConn) ack(id string, ok bool, errMsg string) {
	if id == "" {
		return
	}
	buf, _ := json.Marshal(AckResponse{OK: ok, Error: errMsg})
	_ = c.Write(Envelope{Type: TypeAck, ID: id, Payload: buf})
}