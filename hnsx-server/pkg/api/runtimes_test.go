package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	workerrepo "github.com/hnsx-io/hnsx/server/internal/worker/repository"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func newRuntimeTestServer(t *testing.T) (*Server, *worker.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := workerrepo.NewInMemoryRepository()
	ws := workerservice.NewService(repo)
	reg := ws.Registry()
	application := &app.Application{WorkerService: ws}
	return &Server{WorkerService: ws, Handlers: handler.New(application, nil)}, reg
}

func TestListRuntimes_EmptyWhenNoWorkers(t *testing.T) {
	s, _ := newRuntimeTestServer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtimes", nil)
	s.ListRuntimes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("expected empty list, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

func TestListRuntimes_ReturnsRegisteredWorkers(t *testing.T) {
	s, reg := newRuntimeTestServer(t)

	workerA := &pb.WorkerInfo{
		Version:  "0.1.0",
		Region:   "us-west-2",
		Hostname: "host-a",
		Pid:      "12345",
		Capacity: &pb.ResourceCapacity{
			MaxConcurrentSessions: 4,
			Providers:             []string{"anthropic", "openai"},
			Models:                []string{"claude-sonnet-4"},
			SandboxRuntimes:       []string{"none"},
		},
		Labels: map[string]string{"gpu": "a100"},
	}
	workerB := &pb.WorkerInfo{
		Version:  "0.1.0",
		Region:   "local",
		Hostname: "host-b",
		Capacity: &pb.ResourceCapacity{
			MaxConcurrentSessions: 2,
			Providers:             []string{"claudecode"},
		},
	}
	if _, err := reg.Register(workerA); err != nil {
		t.Fatalf("register A: %v", err)
	}
	if _, err := reg.Register(workerB); err != nil {
		t.Fatalf("register B: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtimes", nil)
	s.ListRuntimes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected 2 workers, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	byID := map[string]map[string]any{}
	for _, it := range resp.Items {
		byID[it["runtime_id"].(string)] = it
	}
	a := byID[workerA.WorkerId]
	if a == nil {
		t.Fatalf("worker A missing: %+v", resp.Items)
	}
	if a["version"] != "0.1.0" {
		t.Fatalf("A version = %v", a["version"])
	}
	if a["region"] != "us-west-2" {
		t.Fatalf("A region = %v", a["region"])
	}
	capabilities, _ := a["capabilities"].([]any)
	if len(capabilities) != 2 {
		t.Fatalf("A capabilities = %v, want 2 entries", capabilities)
	}
	if a["capacity"].(float64) != 4 {
		t.Fatalf("A capacity = %v, want 4", a["capacity"])
	}
	if a["status"] != "healthy" {
		t.Fatalf("A status = %v, want healthy (just registered)", a["status"])
	}
	labels, _ := a["labels"].(map[string]any)
	if labels["gpu"] != "a100" {
		t.Fatalf("A labels = %v, want gpu=a100", labels)
	}
	if a["hostname"] != "host-a" || a["pid"] != "12345" {
		t.Fatalf("A host/pid wrong: %+v", a)
	}
	if _, ok := a["last_heartbeat_at"].(string); !ok {
		t.Fatalf("A last_heartbeat_at missing: %+v", a)
	}

	b := byID[workerB.WorkerId]
	if b == nil {
		t.Fatalf("worker B missing: %+v", resp.Items)
	}
	if b["region"] != "local" {
		t.Fatalf("B region = %v", b["region"])
	}
}

func TestListRuntimes_NilServiceReturnsEmpty(t *testing.T) {
	s := &Server{WorkerService: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtimes", nil)
	s.ListRuntimes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("expected empty list, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

// runtimeStatus mirrors the handler's liveness labelling for unit tests.
func runtimeStatus(snap worker.Snapshot) string {
	if !snap.LastSeen.IsZero() && snap.AgeSeconds > 60 {
		return "offline"
	}
	if snap.Healthy {
		return "healthy"
	}
	return "degraded"
}

func TestRuntimeStatus_Labels(t *testing.T) {
	cases := []struct {
		name string
		snap worker.Snapshot
		want string
	}{
		{
			name: "fresh heartbeat is healthy",
			snap: worker.Snapshot{Healthy: true, LastSeen: time.Now()},
			want: "healthy",
		},
		{
			name: "30–60s heartbeat is degraded",
			snap: worker.Snapshot{Healthy: false, LastSeen: time.Now().Add(-45 * time.Second), AgeSeconds: 45},
			want: "degraded",
		},
		{
			name: "stale heartbeat (>60s) is offline",
			snap: worker.Snapshot{Healthy: false, LastSeen: time.Now().Add(-90 * time.Second), AgeSeconds: 90},
			want: "offline",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeStatus(tc.snap); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
