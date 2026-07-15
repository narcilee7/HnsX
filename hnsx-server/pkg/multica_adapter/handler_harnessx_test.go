package multica_adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestHarnessXRoutesMounted verifies all HarnessX-specific routes are
// registered when the adapter mounts on a gin engine.
func TestHarnessXRoutesMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	wantPaths := []string{
		"/api/harnessx/domains",
		"/api/harnessx/domains/:id",
		"/api/harnessx/domains/:id/run",
		"/api/harnessx/approvals",
		"/api/harnessx/approvals/:id/approve",
		"/api/harnessx/approvals/:id/reject",
		"/api/harnessx/cost/dashboard",
		"/api/harnessx/audit",
	}

	got := map[string]bool{}
	for _, r := range eng.Routes() {
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
			t.Errorf("expected HarnessX route %s to be mounted", p)
		}
	}
}

// TestHarnessXDomains_NoApp verifies the domains endpoint returns an
// empty list when no app is wired (e.g. during tests).
func TestHarnessXDomains_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/harnessx/domains", nil)
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/harnessx/domains status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got []DomainResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(got))
	}
}

// TestHarnessXCostDashboard_NoApp verifies the dashboard returns an empty
// list when no app is wired.
func TestHarnessXCostDashboard_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/harnessx/cost/dashboard", nil)
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/harnessx/cost/dashboard status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got []CostResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("expected empty dashboard; got %d rows", len(got))
	}
}

// TestHarnessXApprovals_NoApp verifies the approvals endpoint.
func TestHarnessXApprovals_NoApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/harnessx/approvals", nil)
	eng.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/harnessx/approvals status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestHarnessXDomainRun_Accepts verifies RunDomain accepts a POST and
// returns 202 when no app is wired.
func TestHarnessXDomainRun_Accepts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	a := &Adapter{daemons: newDaemonRegistry()}
	a.Mount(eng)

	rec := httptest.NewRecorder()
	body := `{"trigger":{"issue_title":"test"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/harnessx/domains/test-domain/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	eng.ServeHTTP(rec, req)

	// Without an app, the handler returns 202 + queued stub.
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/harnessx/domains/x/run status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if got["queued"] != true {
		t.Fatalf("expected queued=true; got %+v", got)
	}
}
