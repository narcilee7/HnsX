package local

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// EmbeddedRunOptions configures a local session executed by the Python worker.
type EmbeddedRunOptions struct {
	// AdapterOverride forces every agent to use this adapter kind.
	// When empty the DomainSpec's declared adapters are used.
	AdapterOverride string

	// PythonPath is the Python interpreter used to run hnsx_worker.session_runtime.
	// When empty FindWorkerPython is used.
	PythonPath string

	// SessionID, TraceID and CorrelationID seed the worker observation IDs.
	SessionID     string
	TraceID       string
	CorrelationID string

	// TimeoutSeconds caps the worker subprocess. Zero means no cap.
	TimeoutSeconds int

	// NoPolicy disables the worker-side policy engine (budget, guardrails,
	// tool allow-lists, approval gates). Useful for local debugging only.
	NoPolicy bool

	// Stdout receives every observation JSON line as it is emitted.
	// Optional; useful for streaming UIs.
	Stdout io.Writer
	// Stderr receives the worker's stderr stream.
	Stderr io.Writer
}

// EmbeddedRunResult is the outcome of a local embedded-worker session.
type EmbeddedRunResult struct {
	State        string           `json:"state"`
	ExitCode     int              `json:"exit_code"`
	Observations []map[string]any `json:"observations"`
	Result       map[string]any   `json:"result,omitempty"`
	Stderr       string           `json:"stderr,omitempty"`
}

// FindWorkerPython locates a Python interpreter capable of running the worker.
// Resolution order:
//  1. HNSX_WORKER_PYTHON environment variable
//  2. A venv next to the running binary: ../hnsx-worker/.venv/bin/python
//  3. A venv in the current working directory or any parent directory
//  4. python3 / python on PATH
func FindWorkerPython() (string, error) {
	if p := os.Getenv("HNSX_WORKER_PYTHON"); p != "" {
		return p, nil
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for _, c := range workerPythonCandidates(exeDir, "..") {
			if isExecutableFile(c) {
				return c, nil
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; dir != "" && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
			for _, c := range workerPythonCandidates(dir, "") {
				if isExecutableFile(c) {
					return c, nil
				}
			}
		}
	}

	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("could not find a Python interpreter for the worker; set HNSX_WORKER_PYTHON")
}

func workerPythonCandidates(base, rel string) []string {
	root := filepath.Join(base, rel)
	return []string{
		filepath.Join(root, "hnsx-worker", ".venv", "bin", "python"),
		filepath.Join(root, "hnsx-worker", ".venv", "Scripts", "python.exe"),
	}
}

func isExecutableFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

// projectRootForPython returns the project root given a worker venv python path.
// It returns an empty string when the path does not look like a project venv.
func projectRootForPython(pythonPath string) string {
	clean := filepath.Clean(pythonPath)
	sep := string(filepath.Separator)
	needle := sep + "hnsx-worker" + sep + ".venv" + sep
	if !strings.Contains(clean, needle) {
		return ""
	}
	// pythonPath is .../hnsx-worker/.venv/bin/python
	// venvDir is .../hnsx-worker/.venv, workerDir is .../hnsx-worker,
	// and the project root is the parent of hnsx-worker.
	venvDir := filepath.Dir(filepath.Dir(clean))
	workerDir := filepath.Dir(venvDir)
	return filepath.Dir(workerDir)
}

// RunEmbeddedSession executes a DomainSpec in a local Python worker subprocess.
// The subprocess runs hnsx_worker.session_runtime with the spec and trigger on stdin.
func RunEmbeddedSession(ctx context.Context, s *spec.DomainSpec, trigger map[string]any, opts EmbeddedRunOptions) (*EmbeddedRunResult, error) {
	python, err := opts.resolvePython()
	if err != nil {
		return nil, err
	}

	projectRoot := projectRootForPython(python)
	if projectRoot == "" {
		if exe, err := os.Executable(); err == nil {
			projectRoot = filepath.Dir(filepath.Dir(exe))
		}
	}
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}

	specJSON, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal domain spec: %w", err)
	}
	triggerJSON, err := json.Marshal(trigger)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger: %w", err)
	}

	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = newEmbeddedSessionID(s.ID)
	}
	traceID := opts.TraceID
	if traceID == "" {
		traceID = sessionID
	}
	correlationID := opts.CorrelationID
	if correlationID == "" {
		correlationID = sessionID
	}

	payload := map[string]any{
		"session_id":           sessionID,
		"trace_id":             traceID,
		"correlation_id":       correlationID,
		"domain_id":            s.ID,
		"domain_spec_json":     string(specJSON),
		"trigger_payload_json": string(triggerJSON),
	}
	if opts.TimeoutSeconds > 0 {
		payload["session_timeout_seconds"] = opts.TimeoutSeconds
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal worker payload: %w", err)
	}

	env := os.Environ()
	if opts.AdapterOverride != "" {
		env = append(env, "HNSX_FORCE_ADAPTER_KIND="+opts.AdapterOverride)
	}
	if opts.NoPolicy {
		env = append(env, "HNSX_DISABLE_POLICY=1")
	}
	env = prependPythonPath(env, projectRoot)

	cmd := exec.CommandContext(ctx, python, "-m", "hnsx_worker.session_runtime")
	cmd.Stdin = strings.NewReader(string(payloadBytes))
	cmd.Dir = projectRoot
	cmd.Env = env

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}

	res := &EmbeddedRunResult{State: "running"}

	stderrDone := make(chan string, 1)
	go func() {
		var sb strings.Builder
		sink := io.Writer(&sb)
		if opts.Stderr != nil {
			sink = io.MultiWriter(&sb, opts.Stderr)
		}
		_, _ = io.Copy(sink, stderrPipe)
		stderrDone <- sb.String()
	}()

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var obs map[string]any
		if err := json.Unmarshal([]byte(line), &obs); err != nil {
			if opts.Stderr != nil {
				fmt.Fprintln(opts.Stderr, line)
			}
			continue
		}
		res.Observations = append(res.Observations, obs)
		if opts.Stdout != nil {
			fmt.Fprintln(opts.Stdout, line)
		}
		if kind, _ := obs["kind"].(string); kind == "session_end" {
			res.State, _ = obs["state"].(string)
			if p, ok := obs["payload"].(map[string]any); ok {
				if r, ok := p["result"].(map[string]any); ok {
					res.Result = r
				}
			}
		}
	}

	stderr := <-stderrDone
	res.Stderr = stderr

	waitErr := cmd.Wait()
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}

	return res, waitErr
}

func prependPythonPath(env []string, root string) []string {
	prefix := "PYTHONPATH="
	for i, e := range env {
		if !strings.HasPrefix(e, prefix) {
			continue
		}
		existing := strings.TrimPrefix(e, prefix)
		merged := root + string(filepath.ListSeparator) + existing
		env[i] = prefix + merged
		return env
	}
	return append(env, prefix+root)
}

func (opts EmbeddedRunOptions) resolvePython() (string, error) {
	if opts.PythonPath != "" {
		return opts.PythonPath, nil
	}
	return FindWorkerPython()
}

func newEmbeddedSessionID(domainID string) string {
	return fmt.Sprintf("%s-%d", domainID, time.Now().UnixNano())
}
