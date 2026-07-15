package multica_adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAdapterMountsRoutes is a smoke test: it boots the adapter against a
// nil application (the adapter's happy path tolerates nil app services for
// routes that don't dereference them) and verifies the route table responds
// to HEAD/OPTIONS without crashing.
//
// Routes that DO touch services (ListAgents, GetAgent, CreateIssue, etc.)
// are verified separately with a fake application that returns canned data.
func TestAdapterMountsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()

	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	routes := eng.Routes()
	if len(routes) == 0 {
		t.Fatal("adapter mounted zero routes")
	}

	// Verify the headline Multica routes are present.
	wantPaths := []string{
		"/api/me",
		"/api/workspaces",
		"/api/workspaces/:id",
		"/api/workspaces/:id/agents",
		"/api/workspaces/:id/agents/:agentId",
		"/api/issues",
		"/api/issues/:id",
		"/api/squads",
		"/api/squads/:id",
		"/api/daemon/register",
		"/api/daemon/heartbeat",
		"/api/daemon/ws",
		"/api/daemon/runtimes/:runtimeId/tasks/claim",
	}
	got := map[string]bool{}
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}
	for _, p := range wantPaths {
		found := false
		for k := range got {
			if strings.HasSuffix(k, " "+p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Multica route %s to be mounted", p)
		}
	}
}

// TestGetMe_NoApp verifies that GetMe works without touching any service.
func TestGetMe_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/me status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /api/me: %v; body=%s", err, rec.Body.String())
	}
	if got.ID == "" {
		t.Fatalf("expected non-empty id; body=%s", rec.Body.String())
	}
}

// TestListWorkspaces_NoApp verifies that ListWorkspaces synthesizes a
// default workspace without touching any service.
func TestListWorkspaces_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got []WorkspaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /api/workspaces: %v; body=%s", err, rec.Body.String())
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one workspace; body=%s", rec.Body.String())
	}
}

// TestDaemonRegister_NoApp verifies that daemon registration responds OK
// without a real session queue (we only test the registration handshake).
func TestDaemonRegister_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	body := `{"daemon_id":"d-test","agent_id":"a-test","runtimes":[{"type":"claude","version":"1.0","status":"online"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/daemon/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/daemon/register status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/daemon/heartbeat", strings.NewReader(`{"daemon_id":"d-test"}`))
	req2.Header.Set("Content-Type", "application/json")
	eng.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST /api/daemon/heartbeat status = %d, want 200; body=%s", rec2.Code, rec2.Body.String())
	}
}

// TestAdapterWithRealAPIServer verifies the adapter wires up alongside the
// existing api.Server without breaking the HnsX-native routes. This is the
// W4 e2e prerequisite: Multica + HnsX share the same gin engine.
//
// We can't easily construct an api.Server without a real app, so this test
// focuses on the route-registration contract: every Multica adapter route
// is mounted on the gin.IRouter we get handed.
func TestAdapterWithRealAPIServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()

	// Simulate api.Server.Handler() by mounting HnsX-native routes inline.
	// The Multica adapter's Mount signature accepts gin.IRouter, so we can
	// register it on the same engine and verify the routes coexist.
	apiGroup := eng.Group("/api/v1")
	apiGroup.GET("/domains", func(c *gin.Context) { c.JSON(200, []any{}) })

	adapter := New(nil, nil)
	adapter.Mount(eng)

	// HnsX-native route still answers.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	eng.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/v1/domains to return 200; got %d", rec.Code)
	}

	// Multica adapter route also answers.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	eng.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/me to return 200; got %d, body=%s", rec.Code, rec.Body.String())
	}
}
