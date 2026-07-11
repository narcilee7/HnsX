package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCsvEscape(t *testing.T) {
	cases := map[string]string{
		"hello":          "hello",
		"a,b":            "\"a,b\"",
		"with\nnewline":  "\"with\nnewline\"",
		`a"b`:            `"a""b"`,
		"":               "",
	}
	for in, want := range cases {
		if got := csvEscape(in); got != want {
			t.Errorf("csvEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	items := []map[string]any{
		{"actor": "alice", "action": "create"},
		{"actor": "bob, jr", "action": "delete", "extra": "with,comma"},
	}
	if err := writeCSV(path, items); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Header + 2 data rows.
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), s)
	}
	if !strings.Contains(lines[0], "actor") || !strings.Contains(lines[0], "action") {
		t.Errorf("header missing keys: %q", lines[0])
	}
	if !strings.Contains(s, `"bob, jr"`) {
		t.Errorf("comma not escaped: %s", s)
	}
}

func TestParseListEnvelope_Items(t *testing.T) {
	body := []byte(`{"items":[{"a":1},{"a":2}],"limit":50,"offset":0,"total":2}`)
	out, err := parseListEnvelope(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out))
	}
}

func TestParseListEnvelope_Array(t *testing.T) {
	body := []byte(`[{"a":1}]`)
	out, err := parseListEnvelope(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
}

func TestParseListEnvelope_Empty(t *testing.T) {
	body := []byte(`{"items":[],"limit":50,"offset":0,"total":0}`)
	out, err := parseListEnvelope(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 items, got %d", len(out))
	}
}

func TestParseListEnvelope_Bad(t *testing.T) {
	if _, err := parseListEnvelope([]byte("not json")); err == nil {
		t.Fatal("expected error on bad json")
	}
}

func TestFilepathHelpers(t *testing.T) {
	if filepathDir("/a/b/c") != "/a/b" {
		t.Fatal("filepathDir wrong")
	}
	if filepathDir("abc") != "." {
		t.Fatal("filepathDir wrong for bare name")
	}
	if filepathJoin("a", "b", "c") != "a/b/c" {
		t.Fatal("filepathJoin wrong")
	}
	if filepathJoin("a/", "b") != "a/b" {
		t.Fatal("filepathJoin should not double-slash")
	}
}

func TestCfgPath(t *testing.T) {
	t.Setenv("HNSX_AUTH_FILE", "/tmp/hnsx-test-auth.yaml")
	if got := cfgPath(); got != "/tmp/hnsx-test-auth.yaml" {
		t.Fatalf("cfgPath = %q", got)
	}
}