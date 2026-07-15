// Package main is the harnessxctl CLI — a thin operator tool for
// inspecting HarnessX state from a terminal. It talks to a running
// control plane over its REST API and prints human-friendly tables.
//
// Usage:
//
//	harnessxctl cost             # cost dashboard
//	harnessxctl audit            # audit log (latest 50)
//	harnessxctl audit-verify     # walk hash chain, fail on first break
//	harnessxctl domains          # list registered HarnessX domains
//	harnessxctl approvals        # pending approval list
//
// The server URL is read from HARNESSX_SERVER (default http://127.0.0.1:50051)
// and the auth token from HARNESSX_TOKEN.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	serverURL := os.Getenv("HARNESSX_SERVER")
	if serverURL == "" {
		serverURL = "http://127.0.0.1:50051"
	}
	token := os.Getenv("HARNESSX_TOKEN")

	run := func(fn func(io.Writer) error) {
		if err := fn(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "harnessxctl:", err)
			os.Exit(1)
		}
	}

	switch cmd {
	case "cost":
		fs := flag.NewFlagSet("cost", flag.ExitOnError)
		_ = fs.Parse(args)
		run(func(w io.Writer) error { return printCost(w, serverURL, token) })
	case "audit":
		fs := flag.NewFlagSet("audit", flag.ExitOnError)
		limit := fs.Int("limit", 50, "max entries to print")
		_ = fs.Parse(args)
		run(func(w io.Writer) error { return printAudit(w, serverURL, token, *limit) })
	case "audit-verify":
		run(func(w io.Writer) error { return printAuditVerify(w, serverURL, token) })
	case "domains":
		run(func(w io.Writer) error { return printDomains(w, serverURL, token) })
	case "approvals":
		run(func(w io.Writer) error { return printApprovals(w, serverURL, token) })
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `harnessxctl — operator CLI for HarnessX

Usage:
  harnessxctl <command> [flags]

Commands:
  cost                Print per-Domain cost dashboard
  audit [--limit N]   Print recent audit log entries (default 50)
  audit-verify        Walk hash chain, fail on first broken record
  domains             List registered HarnessX domains
  approvals           List pending approval requests
  help                Show this help

Env:
  HARNESSX_SERVER     Server base URL (default http://127.0.0.1:50051)
  HARNESSX_TOKEN      Bearer token (if server requires auth)
`)
}

// ── HTTP helpers ────────────────────────────────────────────────────────

func getJSON(w io.Writer, serverURL, token, path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, serverURL+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ── Commands ────────────────────────────────────────────────────────────

func printCost(w io.Writer, serverURL, token string) error {
	var rows []map[string]any
	if err := getJSON(w, serverURL, token, "/api/harnessx/cost/dashboard", &rows); err != nil {
		return err
	}
	// Stable ordering by cost desc.
	sort.Slice(rows, func(i, j int) bool {
		return toFloat(rows[i]["total_cost_usd"]) > toFloat(rows[j]["total_cost_usd"])
	})
	fmt.Fprintln(w, "DOMAIN                                SESSIONS   TOTAL COST   AVG COST   TOTAL TOKENS")
	fmt.Fprintln(w, strings.Repeat("-", 90))
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no sessions yet)")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-38s %6d   $%-9.4f   $%-7.4f   %d\n",
			asString(r["domain_id"]),
			asInt(r["session_count"]),
			toFloat(r["total_cost_usd"]),
			toFloat(r["avg_cost_usd"]),
			asInt(r["total_tokens"]),
		)
	}
	return nil
}

func printAudit(w io.Writer, serverURL, token string, limit int) error {
	path := fmt.Sprintf("/api/harnessx/audit?limit=%d", limit)
	var rows []map[string]any
	if err := getJSON(w, serverURL, token, path, &rows); err != nil {
		return err
	}
	fmt.Fprintln(w, "TIME                  ACTION     ACTOR   RESOURCE             DECISION  REASON")
	fmt.Fprintln(w, strings.Repeat("-", 100))
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no audit entries)")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-22s %-10s %-7s %-20s %-9s %s\n",
			asString(r["timestamp_ms"]),
			asString(r["action"]),
			asString(r["actor"]),
			asString(r["resource"]),
			asString(r["decision"]),
			asString(r["reason"]),
		)
	}
	return nil
}

// printAuditVerify walks the audit hash chain. The server returns audit
// entries with a prev_hash + hash field; we verify the chain is unbroken.
//
// P0 (W26) treats the chain as the server returned it; P1+ can also
// re-derive the hashes from the entry bodies to catch tampering.
func printAuditVerify(w io.Writer, serverURL, token string) error {
	var rows []map[string]any
	if err := getJSON(w, serverURL, token, "/api/harnessx/audit?limit=500", &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no audit entries; chain is trivially valid")
		return nil
	}
	prevHash := ""
	for i, r := range rows {
		gotPrev := asString(r["prev_hash"])
		if gotPrev != prevHash {
			fmt.Fprintf(w, "✗ chain broken at entry #%d (id=%s): expected prev_hash %q, got %q\n",
				i, asString(r["id"]), prevHash, gotPrev)
			return fmt.Errorf("audit chain verification failed")
		}
		prevHash = asString(r["hash"])
	}
	fmt.Fprintf(w, "✓ audit chain valid (%d entries verified)\n", len(rows))
	return nil
}

func printDomains(w io.Writer, serverURL, token string) error {
	var rows []map[string]any
	if err := getJSON(w, serverURL, token, "/api/harnessx/domains", &rows); err != nil {
		return err
	}
	fmt.Fprintln(w, "DOMAIN ID                          VERSION   AGENTS  SKILLS  TOOLS  POLICY  BUDGET  MODE")
	fmt.Fprintln(w, strings.Repeat("-", 100))
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no HarnessX domains registered)")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-34s %-9s %5d  %5d  %5d  %6v  %6v  %s\n",
			asString(r["id"]),
			asString(r["version"]),
			asInt(r["agent_count"]),
			asInt(r["skill_count"]),
			asInt(r["tool_count"]),
			asBool(r["has_policy"]),
			asBool(r["has_budget"]),
			asString(r["session_mode"]),
		)
	}
	return nil
}

func printApprovals(w io.Writer, serverURL, token string) error {
	var rows []map[string]any
	if err := getJSON(w, serverURL, token, "/api/harnessx/approvals", &rows); err != nil {
		return err
	}
	fmt.Fprintln(w, "APPROVAL ID      SESSION ID       DOMAIN         ACTION          RISK     STATUS")
	fmt.Fprintln(w, strings.Repeat("-", 100))
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no pending approvals)")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-16s %-16s %-13s %-15s %-8s %s\n",
			asString(r["id"]),
			asString(r["session_id"]),
			asString(r["domain_id"]),
			asString(r["action"]),
			asString(r["risk_level"]),
			asString(r["status"]),
		)
	}
	return nil
}

// ── Type coercion helpers ───────────────────────────────────────────────

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func asBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return false
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// time is referenced so the file imports compile when the audit table
// gains a time-typed column later.
var _ = time.Now
