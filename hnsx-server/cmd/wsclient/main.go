// Command wsclient is a smoke test for the /ws/daemon endpoint.
//
// Connects, sends a claim envelope, reads the response, posts a
// batch of observations, and exits. Useful for end-to-end verification
// of the daemon ↔ server WS protocol.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hnsx-io/hnsx/server/internal/ws"
)

func main() {
	url := flag.String("url", "ws://127.0.0.1:8080/ws/daemon", "ws URL")
	workspaceID := flag.String("workspace", "", "workspace ID")
	flag.Parse()

	if *workspaceID == "" {
		fmt.Fprintln(os.Stderr, "usage: wsclient -workspace <id>")
		os.Exit(2)
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := dialer.DialContext(ctx, *url, http.Header{})
	if err != nil {
		log.Fatalf("dial: %v (status=%v)", err, resp)
	}
	defer conn.Close()
	log.Printf("ws: connected to %s", *url)

	// 1. claim
	claim := ws.ClaimRequest{WorkspaceID: *workspaceID, DaemonName: "smoke", MaxItems: 10}
	buf, _ := json.Marshal(claim)
	if err := conn.WriteJSON(ws.Envelope{Type: ws.TypeClaim, ID: "c1", Payload: buf}); err != nil {
		log.Fatalf("write claim: %v", err)
	}
	log.Printf("ws: sent claim")

	// 2. read issues response
	var respEnv ws.Envelope
	if err := conn.ReadJSON(&respEnv); err != nil {
		log.Fatalf("read issues: %v", err)
	}
	if respEnv.Type != ws.TypeIssues {
		log.Fatalf("unexpected response type %q", respEnv.Type)
	}
	var issues ws.IssuesResponse
	if err := json.Unmarshal(respEnv.Payload, &issues); err != nil {
		log.Fatalf("decode issues: %v", err)
	}
	log.Printf("ws: got %d issues", len(issues.Items))
	for _, i := range issues.Items {
		log.Printf("  - %s  %s  (agent=%s)", i.ID, truncate(i.Title, 40), i.AgentID)
	}

	// 3. post a single test observation (use the first issue if any)
	if len(issues.Items) > 0 {
		iss := issues.Items[0]
		obs := ws.ObservationEvent{
			WorkspaceID: iss.WorkspaceID,
			IssueID:     iss.ID,
			AgentID:     iss.AgentID,
			Kind:        "message",
			Sequence:    1,
			Payload:     json.RawMessage(`{"type":"system","subtype":"ws_smoke"}`),
			OccurredAt:  time.Now().UTC().Format(time.RFC3339),
			PromptHash:  "ws_smoke",
			ToolSignatures: json.RawMessage(`["ws_smoke"]`),
		}
		buf, _ := json.Marshal([]ws.ObservationEvent{obs})
		if err := conn.WriteJSON(ws.Envelope{Type: ws.TypeObservations, ID: "o1", Payload: buf}); err != nil {
			log.Fatalf("write observations: %v", err)
		}
		var ackEnv ws.Envelope
		if err := conn.ReadJSON(&ackEnv); err != nil {
			log.Fatalf("read ack: %v", err)
		}
		if ackEnv.Type != ws.TypeAck {
			log.Fatalf("expected ack, got %q", ackEnv.Type)
		}
		var ack ws.AckResponse
		_ = json.Unmarshal(ackEnv.Payload, &ack)
		log.Printf("ws: observations ack ok=%v err=%q", ack.OK, ack.Error)
	}

	// 4. heartbeat
	hb, _ := json.Marshal(map[string]any{"workspace_id": *workspaceID})
	if err := conn.WriteJSON(ws.Envelope{Type: ws.TypeHeartbeat, ID: "h1", Payload: hb}); err != nil {
		log.Fatalf("write heartbeat: %v", err)
	}
	var ackEnv ws.Envelope
	if err := conn.ReadJSON(&ackEnv); err != nil {
		log.Fatalf("read hb ack: %v", err)
	}
	log.Printf("ws: heartbeat ack type=%s", ackEnv.Type)

	// 5. close
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	log.Printf("ws: done")
	_ = bytes.Buffer{}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}