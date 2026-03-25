package pipeline

import (
	"fmt"
	"strings"
)

// Interpolate replaces all {{key}} and {{dot.path}} placeholders in s with
// values from vars. For dot-separated keys like {{s1.out}}, it walks the
// nested map structure. Values of any type are coerced to string via fmt.Sprint.
// Unknown keys are left unchanged.
func Interpolate(s string, vars map[string]any) string {
	// Replace each {{...}} placeholder.
	var result strings.Builder
	remaining := s
	for {
		start := strings.Index(remaining, "{{")
		if start == -1 {
			result.WriteString(remaining)
			break
		}
		end := strings.Index(remaining[start:], "}}")
		if end == -1 {
			result.WriteString(remaining)
			break
		}
		end += start

		result.WriteString(remaining[:start])
		key := remaining[start+2 : end]

		if v, ok := resolvePath(vars, key); ok {
			result.WriteString(fmt.Sprint(v))
		} else {
			// Leave placeholder unchanged.
			result.WriteString("{{")
			result.WriteString(key)
			result.WriteString("}}")
		}

		remaining = remaining[end+2:]
	}
	return result.String()
}

// resolvePath looks up key in vars. It first tries an exact key match (supporting
// keys with literal dots like "s1.out"), then falls back to dot-path traversal
// for nested maps ("step.fetch.data.url" → vars["step"]["fetch"]["data"]["url"]).
func resolvePath(vars map[string]any, key string) (any, bool) {
	// Exact key match first (handles flat keys like "s1.out").
	if v, ok := vars[key]; ok {
		return v, true
	}
	// Fall back to hierarchical dot-path traversal.
	if !strings.Contains(key, ".") {
		return nil, false
	}
	parts := strings.SplitN(key, ".", 2)
	v, ok := vars[parts[0]]
	if !ok {
		return nil, false
	}
	nested, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	return resolvePath(nested, parts[1])
}
