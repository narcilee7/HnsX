package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validDomainYAML is the minimum DomainSpec that passes `hnsx validate`.
// Kept here so the deploy tests don't have to invent a different shape
// per case — anything more elaborate isn't testing deploy, it's testing
// the validator.
const validDomainYAML = `id: my-cs
version: 0.1.0
description: |
  Customer service triage harness used by deploy tests.
harness:
  agents:
    main:
      id: main
      provider: noop
      model: noop-1
      adapter:
        kind: noop
  prompts:
    default:
      type: system
      template: "you are a helper"
  session:
    mode: single
    agent: main
`

// TestDeployCmd_RequiresDomainYAML verifies a friendly error when the
// user passes a path that doesn't exist.
func TestDeployCmd_RequiresDomainYAML(t *testing.T) {
	cfg := Default()
	cmd := newDeployCmd(&cfg)
	cmd.SetArgs([]string{"does-not-exist.yaml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing domain.yaml")
	}
}

// TestDeployCmd_RejectsUnknownTarget verifies --target validation.
// (This runs before validateFile because we don't need a valid YAML to
// hit the target switch.)
func TestDeployCmd_RejectsUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	if err := os.WriteFile(path, []byte(validDomainYAML), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg := Default()
	cmd := newDeployCmd(&cfg)
	cmd.SetArgs([]string{path, "--target", "satellite"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

// TestDeployCmd_ServerUnreachableWithoutUp verifies the "use --up" hint.
func TestDeployCmd_ServerUnreachableWithoutUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	if err := os.WriteFile(path, []byte(validDomainYAML), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg := Default()
	cfg.ServerURL = "http://127.0.0.1:1" // unreachable port
	cmd := newDeployCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when server unreachable without --up")
	}
	out := buf.String()
	if !strings.Contains(out, "Pass --up") {
		t.Errorf("expected output to mention --up, got:\n%s", out)
	}
}

// TestDeployCmd_LocalHappyPath exercises the full deploy against a fake
// hnsx-server. The internal client uses Connect RPC, so we mock both the
// HTTP /healthz endpoint and the Connect-style service paths.
//
// The Connect RPC over HTTP wire format used by hnsx-server is:
//
//   POST /hnsx.v1.DomainRegistryService/RegisterDomain
//   Content-Type: application/json
//   Connect-Protocol-Version: 1
//   {"spec": {...}}
//   -> 200 {"domain": {...}}
//
// We don't parse the JSON body here — we only assert that the path was
// hit. The full round-trip is exercised by the smoke tests in
// scripts/smoke.sh against a real server.
func TestDeployCmd_LocalHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	if err := os.WriteFile(path, []byte(validDomainYAML), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var registeredPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/domains/"):
			// Domain doesn't exist yet.
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/domains":
			registeredPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"my-cs","version":"0.1.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := Default()
	cfg.ServerURL = srv.URL
	cmd := newDeployCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("deploy: %v\noutput:\n%s", err, buf.String())
	}
	if registeredPath != "/api/v1/domains" {
		t.Errorf("expected POST /api/v1/domains, got %q", registeredPath)
	}
	if !strings.Contains(buf.String(), "Deployed my-cs") {
		t.Errorf("expected success message, got:\n%s", buf.String())
	}
}

// TestDeployCmd_RejectsExistingDomainWithoutForce: if the server already
// has the domain, deploy refuses and tells the user to pass --force.
func TestDeployCmd_RejectsExistingDomainWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	if err := os.WriteFile(path, []byte(validDomainYAML), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/domains/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"my-cs","version":"0.0.9"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := Default()
	cfg.ServerURL = srv.URL
	cmd := newDeployCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{path})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when domain already registered without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
	if !strings.Contains(buf.String(), "Pass --force") {
		t.Errorf("expected --force hint, got:\n%s", buf.String())
	}
}

// TestReadDomainIDAndVersion exercises the lightweight YAML reader.
func TestReadDomainIDAndVersion(t *testing.T) {
	cases := []struct {
		name        string
		yaml        string
		wantID      string
		wantVersion string
		wantErr     bool
	}{
		{"both", "id: foo\nversion: 1.2.3\n", "foo", "1.2.3", false},
		{"only id", "id: bar\n", "bar", "", false},
		{"missing id", "version: 1.0.0\n", "", "", true},
		{"empty", "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "d.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("seed: %v", err)
			}
			id, ver, err := readDomainIDAndVersion(path)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			if ver != tc.wantVersion {
				t.Errorf("version = %q, want %q", ver, tc.wantVersion)
			}
		})
	}
}

// TestServerReachable verifies the health-check helper against a
// stand-up httptest server (true case) and a closed one (false case).
func TestServerReachable(t *testing.T) {
	// Healthy server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cfg := Default()
	cfg.ServerURL = srv.URL
	if !serverReachable(&cfg) {
		t.Errorf("expected serverReachable=true against %s", srv.URL)
	}
	srv.Close()

	// Closed server: re-use the loopback port; the server is gone, so
	// we should hit "connection refused".
	cfg.ServerURL = srv.URL
	if serverReachable(&cfg) {
		t.Errorf("expected serverReachable=false after srv.Close")
	}
}

// TestDeployCmd_CloudNotImplemented: hitting --target cloud before the
// cloud backend exists should print a clear "not yet implemented" error
// and a fallback hint, regardless of whether `gh` is installed.
func TestDeployCmd_CloudNotImplemented(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	if err := os.WriteFile(path, []byte(validDomainYAML), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := Default()
	cmd := newDeployCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{path, "--target", "cloud"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for cloud target (not implemented yet)")
	}
	out := buf.String()
	if !strings.Contains(out, "not yet implemented") {
		t.Errorf("expected 'not yet implemented' hint, got:\n%s", out)
	}
	if !strings.Contains(out, "--target local") {
		t.Errorf("expected fallback hint to --target local, got:\n%s", out)
	}
}