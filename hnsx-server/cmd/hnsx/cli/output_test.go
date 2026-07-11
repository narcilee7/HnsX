package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestOutput_Print_JSON(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "json", w: &buf}
	o.Print(map[string]any{"k": "v"})
	if !strings.Contains(buf.String(), "\"k\": \"v\"") {
		t.Fatalf("expected JSON output, got %q", buf.String())
	}
}

func TestOutput_Print_Quiet(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "quiet", w: &buf}
	o.Print(map[string]any{"k": "v"})
	if buf.Len() != 0 {
		t.Fatalf("quiet mode should suppress, got %q", buf.String())
	}
}

func TestOutput_Line_Human(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "human", w: &buf}
	o.Line("hello %s", "world")
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Fatalf("Line got %q", got)
	}
}

func TestOutput_Table(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "human", w: &buf}
	o.Table([]string{"A", "B"}, [][]string{{"1", "2"}, {"3", "4"}})
	out := buf.String()
	for _, want := range []string{"A", "B", "1", "2", "3", "4", "-"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q in:\n%s", want, out)
		}
	}
}

func TestOutput_Table_JSON(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "json", w: &buf}
	o.Table([]string{"A"}, [][]string{{"x"}, {"y"}})
	var got []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(got) != 2 || got[0]["A"] != "x" || got[1]["A"] != "y" {
		t.Fatalf("unexpected table JSON: %+v", got)
	}
}

func TestOutput_Table_Quiet(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "quiet", w: &buf}
	o.Table([]string{"A"}, [][]string{{"x"}})
	if buf.Len() != 0 {
		t.Fatalf("quiet should suppress table, got %q", buf.String())
	}
}

func TestOutput_KV_Human(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "human", w: &buf}
	o.KV([][2]string{{"id", "abc"}, {"state", "running"}})
	out := buf.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "abc") {
		t.Fatalf("KV missing id/abc: %q", out)
	}
	if !strings.Contains(out, "state") || !strings.Contains(out, "running") {
		t.Fatalf("KV missing state/running: %q", out)
	}
}

func TestOutput_KV_JSON(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "json", w: &buf}
	o.KV([][2]string{{"id", "abc"}, {"state", "running"}})
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if got["id"] != "abc" || got["state"] != "running" {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestOutput_KV_Quiet(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "quiet", w: &buf}
	o.KV([][2]string{{"id", "abc"}})
	if buf.Len() != 0 {
		t.Fatalf("quiet should suppress KV, got %q", buf.String())
	}
}

func TestOutput_Card_Human(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "human", w: &buf}
	o.Card("Session", [][2]string{{"id", "abc"}})
	out := buf.String()
	if !strings.Contains(out, "Session") {
		t.Fatalf("card missing title: %q", out)
	}
	if !strings.Contains(out, "abc") {
		t.Fatalf("card missing value: %q", out)
	}
}

func TestOutput_Section_Human(t *testing.T) {
	var buf bytes.Buffer
	o := &Output{mode: "human", w: &buf}
	o.Section("Trigger")
	out := buf.String()
	if !strings.Contains(out, "Trigger") {
		t.Fatalf("section missing title: %q", out)
	}
}
