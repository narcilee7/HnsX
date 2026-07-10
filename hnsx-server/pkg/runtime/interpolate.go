package runtime

import "encoding/json"

// interpolateValue decodes raw JSON into a generic value, then expands
// ${var} placeholders recursively against vars.
func interpolateValue(raw json.RawMessage, vars map[string]any) any {
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

// expandString replaces `${var}` occurrences with their JSON-encoded values.
// Unknown placeholders are left as-is so downstream surfaces can flag them.
func expandString(s string, vars map[string]any) string {
	out := s
	for {
		start := -1
		end := -1
		for i := 0; i+2 < len(out); i++ {
			if out[i] == '$' && out[i+1] == '{' {
				start = i
				j := i + 2
				for j < len(out) && out[j] != '}' {
					j++
				}
				if j < len(out) {
					end = j + 1
					break
				}
			}
		}
		if start < 0 {
			return out
		}
		key := out[start+2 : end-1]
		if v, ok := vars[key]; ok {
			b, _ := json.Marshal(v)
			out = out[:start] + string(b) + out[end:]
		} else {
			// Leave the placeholder in place.
			return out
		}
	}
}
