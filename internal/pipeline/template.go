package pipeline

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// stepInputPattern matches {{ steps.<id>.<key> }} template expressions.
var stepInputPattern = regexp.MustCompile(`\{\{\s*steps\.([^.}\s]+)\.([^}\s]+)\s*\}\}`)

// Interpolate replaces all {{key}} and {{dot.path}} placeholders in s with
// values from vars. For dot-separated keys like {{step.fetch.data.url}}, it
// walks the nested map structure. Values of any type are coerced to string via
// fmt.Sprint. Unknown keys are left unchanged.
//
// Legacy backwards-compat: {{<ID>.out}} where <ID> contains no dots is first
// tried as a flat key lookup (e.g. vars["s1.out"]), then as step.<ID>.data.value
// in the nested map. A deprecation warning is logged on the second path.
func Interpolate(s string, vars map[string]any) string {
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

		if v, ok := resolveKey(vars, key); ok {
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

// resolveKey resolves a template key against vars. Resolution order:
//  1. Exact flat key (e.g. "s1.out" stored as a top-level key).
//  2. Hierarchical dot-path traversal (e.g. "step.fetch.data.url").
//  3. Legacy shim: if the key is "<ID>.out" with no dots in <ID>,
//     try "step.<ID>.data.value" with a deprecation warning.
func resolveKey(vars map[string]any, key string) (any, bool) {
	// 1. Exact flat key.
	if v, ok := vars[key]; ok {
		return v, true
	}

	// 2. Hierarchical dot-path traversal.
	if v, ok := resolvePath(vars, key); ok {
		return v, true
	}

	// 3. Legacy shim: {{<ID>.out}} → step.<ID>.data.value
	if strings.HasSuffix(key, ".out") {
		id := strings.TrimSuffix(key, ".out")
		if !strings.Contains(id, ".") {
			newKey := "step." + id + ".data.value"
			if v, ok := resolvePath(vars, newKey); ok {
				log.Printf("[pipeline] DEPRECATED: {{%s}} — use {{%s}} instead", key, newKey)
				return v, true
			}
		}
	}

	return nil, false
}

// ResolveStepInputs resolves all {{ steps.<id>.<key> }} template expressions in s
// using accumulated step outputs from ec. Returns an error if any referenced output
// does not exist (step ID or key not found).
func ResolveStepInputs(s string, ec *ExecutionContext, stepName, runIDStr string) (string, error) {
	var resolveErr error
	result := stepInputPattern.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}
		subs := stepInputPattern.FindStringSubmatch(match)
		if len(subs) < 3 {
			return match
		}
		stepID, key := subs[1], subs[2]
		val, ok := ec.StepOutput(stepID, key)
		if !ok {
			resolveErr = fmt.Errorf("step %s: input %q: step %s output %s not found (run_id=%s, step=%s)",
				stepName, match, stepID, key, runIDStr, stepName)
			return match
		}
		return val
	})
	return result, resolveErr
}

// resolvePath looks up key in vars by walking the nested map using dot-path
// traversal. E.g. "step.fetch.data.url" → vars["step"]["fetch"]["data"]["url"].
func resolvePath(vars map[string]any, key string) (any, bool) {
	if !strings.Contains(key, ".") {
		v, ok := vars[key]
		return v, ok
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
