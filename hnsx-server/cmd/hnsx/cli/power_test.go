package cli

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDiffStrings(t *testing.T) {
	var out []diffChange
	diffStrings("id", "a", "a", &out)
	if len(out) != 0 {
		t.Fatalf("equal values should not produce a change")
	}
	diffStrings("id", "a", "b", &out)
	if len(out) != 1 || out[0].Section != "id" {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestDiffAny(t *testing.T) {
	var out []diffChange
	diffAny("len", 3, 3, &out)
	if len(out) != 0 {
		t.Fatal("equal values should not produce a change")
	}
	diffAny("len", 3, 4, &out)
	if len(out) != 1 {
		t.Fatal("expected one change")
	}
	diffAny("complex", []int{1, 2}, []int{1, 2}, &out)
	if len(out) != 1 { // unchanged from prior
		t.Fatalf("unexpected count: %d", len(out))
	}
}

func TestSortMapKeys(t *testing.T) {
	src := `
z: 1
a: 2
m:
  z: 1
  a: 2
nested:
  arr:
    - z
    - a
`
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	sortMapKeys(&n)
	out, err := yaml.Marshal(&n)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	// The top-level keys should be ordered a < m < z.
	ia := indexOf(got, "a: 2")
	im := indexOf(got, "m:\n")
	iz := indexOf(got, "z: 1")
	if !(ia >= 0 && im > ia && iz > im) {
		t.Fatalf("expected a < m < z in:\n%s", got)
	}
}

func TestMapKeysToJSON(t *testing.T) {
	b, err := mapKeysToJSON(map[string]int{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Must contain both keys; order is map-iteration-defined.
	if indexOf(s, "\"a\": 1") < 0 || indexOf(s, "\"b\": 2") < 0 {
		t.Fatalf("missing keys: %s", s)
	}
}