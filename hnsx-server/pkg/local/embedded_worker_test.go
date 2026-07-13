package local

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func canImportHnsxWorker(python string) bool {
	cmd := exec.Command(python, "-c", "import hnsx_worker")
	return cmd.Run() == nil
}

func TestFindWorkerPythonFallback(t *testing.T) {
	// When HNSX_WORKER_PYTHON is set it wins.
	t.Setenv("HNSX_WORKER_PYTHON", "/fake/python")
	p, err := FindWorkerPython()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != "/fake/python" {
		t.Fatalf("expected env override, got %s", p)
	}
}

func TestRunEmbeddedSessionNoopSmoke(t *testing.T) {
	python, err := FindWorkerPython()
	if err != nil {
		t.Skipf("worker python not available: %v", err)
	}
	if !canImportHnsxWorker(python) {
		t.Skipf("python %s cannot import hnsx_worker; install the worker package first", python)
	}

	root, err := findProjectRoot(python)
	if err != nil {
		t.Skipf("cannot locate project root: %v", err)
	}

	domainPath := filepath.Join(root, "example-domains", "noop-smoke", "domain.yaml")
	if _, err := os.Stat(domainPath); err != nil {
		t.Skipf("example domain not found: %v", err)
	}

	s, err := domain.LoadFile(domainPath)
	if err != nil {
		t.Fatalf("load domain: %v", err)
	}

	res, err := RunEmbeddedSession(context.Background(), s, map[string]any{}, EmbeddedRunOptions{
		AdapterOverride: "noop",
		PythonPath:      python,
	})
	if err != nil {
		t.Fatalf("run embedded session: %v\nstderr:\n%s", err, res.Stderr)
	}

	if res.State != "completed" {
		t.Fatalf("expected state completed, got %s", res.State)
	}
	if len(res.Observations) == 0 {
		t.Fatal("expected observations")
	}
	if res.Result["output"] == "" {
		t.Fatal("expected non-empty result output")
	}
}

func TestRunEmbeddedSessionWorkflowEcho(t *testing.T) {
	python, err := FindWorkerPython()
	if err != nil {
		t.Skipf("worker python not available: %v", err)
	}
	if !canImportHnsxWorker(python) {
		t.Skipf("python %s cannot import hnsx_worker; install the worker package first", python)
	}

	root, err := findProjectRoot(python)
	if err != nil {
		t.Skipf("cannot locate project root: %v", err)
	}

	domainPath := filepath.Join(root, "example-domains", "workflow-demo", "domain.yaml")
	if _, err := os.Stat(domainPath); err != nil {
		t.Skipf("example domain not found: %v", err)
	}

	s, err := domain.LoadFile(domainPath)
	if err != nil {
		t.Fatalf("load domain: %v", err)
	}

	res, err := RunEmbeddedSession(context.Background(), s, map[string]any{"question": "hello"}, EmbeddedRunOptions{
		AdapterOverride: "echo",
		PythonPath:      python,
	})
	if err != nil {
		t.Fatalf("run embedded session: %v\nstderr:\n%s", err, res.Stderr)
	}

	if res.State != "completed" {
		t.Fatalf("expected state completed, got %s", res.State)
	}

	var sawStepStart bool
	for _, obs := range res.Observations {
		if obs["kind"] == "step_start" {
			sawStepStart = true
			break
		}
	}
	if !sawStepStart {
		t.Fatal("expected at least one step_start observation for workflow mode")
	}
}

func findProjectRoot(python string) (string, error) {
	if root := projectRootForPython(python); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// The Go module lives under hnsx-server/, but the repo root (which holds
	// example-domains/) is one level above. Walk up and prefer the directory
	// that actually contains the example domains.
	for dir := cwd; dir != "" && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "example-domains", "noop-smoke", "domain.yaml")); err == nil {
			return dir, nil
		}
	}
	// Fallback: repo root identified by go.work.
	for dir := cwd; dir != "" && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
	}
	return cwd, nil
}

func TestRunEmbeddedSessionNoPolicy(t *testing.T) {
	python, err := FindWorkerPython()
	if err != nil {
		t.Skipf("worker python not available: %v", err)
	}
	if !canImportHnsxWorker(python) {
		t.Skipf("python %s cannot import hnsx_worker; install the worker package first", python)
	}

	root, err := findProjectRoot(python)
	if err != nil {
		t.Skipf("cannot locate project root: %v", err)
	}

	domainPath := filepath.Join(root, "example-domains", "noop-smoke", "domain.yaml")
	if _, err := os.Stat(domainPath); err != nil {
		t.Skipf("example domain not found: %v", err)
	}

	s, err := domain.LoadFile(domainPath)
	if err != nil {
		t.Fatalf("load domain: %v", err)
	}

	res, err := RunEmbeddedSession(context.Background(), s, map[string]any{}, EmbeddedRunOptions{
		AdapterOverride: "noop",
		PythonPath:      python,
		NoPolicy:        true,
	})
	if err != nil {
		t.Fatalf("run embedded session: %v\nstderr:\n%s", err, res.Stderr)
	}

	if res.State != "completed" {
		t.Fatalf("expected state completed, got %s", res.State)
	}
	for _, obs := range res.Observations {
		if obs["kind"] == "policy_check" {
			t.Fatal("expected no policy_check observations with NoPolicy=true")
		}
	}
}
