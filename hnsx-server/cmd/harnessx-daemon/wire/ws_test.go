package wire

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/config"
)

// TestWSClient_Roundtrip verifies the WS client can connect to a server,
// send a message, and receive a server-pushed event back.
func TestWSClient_Roundtrip(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	gotEvent := make(chan ServerEvent, 1)

	// Spin up a fake server with /api/daemon/ws that echoes back a
	// ServerEvent after the client sends one.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/daemon/ws" {
			http.NotFound(w, r)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Read first frame from client (Register or test payload).
		_, raw, err := c.ReadMessage()
		if err != nil {
			return
		}
		var incoming Message
		_ = json.Unmarshal(raw, &incoming)
		// Push a server-pushed event back to the client.
		payload, _ := json.Marshal(ServerEvent{
			DaemonID: "server-pushed",
		})
		out := Message{Type: "daemon.daemon_id", Payload: payload}
		raw, _ = json.Marshal(out)
		_ = c.WriteMessage(websocket.TextMessage, raw)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.ServerURL = srv.URL
	cfg.WorkspaceID = "ws-test"

	w := NewWSClient(cfg)
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Send a register-style event.
	if err := w.Send(ServerEvent{DaemonID: "d-test"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Drain the read loop in the background, capture the first event.
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_ = w.ReadLoop(readCtx, func(e ServerEvent) {
		select {
		case gotEvent <- e:
		default:
		}
	})

	select {
	case e := <-gotEvent:
		if e.DaemonID != "server-pushed" {
			t.Fatalf("expected DaemonID=server-pushed; got %q", e.DaemonID)
		}
		if !strings.HasPrefix(e.Kind, "daemon.") {
			t.Fatalf("expected Kind prefix daemon.*; got %q", e.Kind)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server event")
	}
}
