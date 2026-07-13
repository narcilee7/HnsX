// Variable interpolation — moved from pkg/runtime/interpolate.go in Phase 3.

package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InterpolateValue decodes raw JSON into a generic value, then expands
// ${var} placeholders recursively against vars.
func InterpolateValue(raw json.RawMessage, vars map[string]any) any {
	if len(raw) == 0 {
		return nil
	}
	var anyVal any
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		return string(raw)
	}
	return walk(anyVal, vars)
}

// walk recursively descends any value, expanding `${var}` placeholders when it
// encounters strings.
func walk(v any, vars map[string]any) any {
	switch val := v.(type) {
	case string:
		return expandString(val, vars)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = walk(item, vars)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = walk(item, vars)
		}
		return out
	default:
		return val
	}
}

// expandString replaces `${var}` placeholders in s using vars. Unknown
// placeholders are left untouched so debugging is easier.
func expandString(s string, vars map[string]any) string {
	if len(vars) == 0 || len(s) < 3 {
		return s
	}
	// Simple linear scan; perf is fine for spec-sized strings.
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+2 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := -1
			for j := i + 2; j < len(s); j++ {
				if s[j] == '}' {
					end = j
					break
				}
				if s[j] == '\n' {
					break
				}
			}
			if end > 0 {
				key := s[i+2 : end]
				if v, ok := vars[key]; ok {
					out.WriteString(toString(v))
				} else {
					out.WriteString(s[i : end+1])
				}
				i = end + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
