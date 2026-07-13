package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainFormatCmd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain.yaml")
	src := `id: z-domain
version: 0.1.0
harness:
  agents:
    b:
      id: b
      provider: noop
    a:
      id: a
      provider: noop
  session:
    mode: single
    agent: a
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(func() error { return formatOne(path, false, NewOutput("human")) })
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	// Sorted keys: a should come before b.
	ia := strings.Index(out, "a:\n")
	ib := strings.Index(out, "b:\n")
	if ia < 0 || ib < 0 || ib < ia {
		t.Fatalf("expected sorted agent keys a < b in:\n%s", out)
	}
}

func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	execErr := fn()
	w.Close()
	os.Stdout = old

	data, readErr := io.ReadAll(r)
	if readErr != nil {
		return "", readErr
	}
	return string(data), execErr
}
