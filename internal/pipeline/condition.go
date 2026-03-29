package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EvalCondition evaluates a condition expression against the execution context.
// The "_output" key in vars is used as the primary value for expression matching.
// Supported expressions:
//   - "always"         → always true
//   - "contains:<str>" → true if output contains str
//   - "matches:<re>"   → true if output matches the regex
//   - "len > <n>"      → true if len(output) > n
func EvalCondition(expr string, vars map[string]any) bool {
	expr = strings.TrimSpace(expr)

	// Extract the primary output value from context (stored as step's raw output).
	output := ""
	if v, ok := vars["_output"]; ok {
		output = toString(v)
	}

	switch {
	case expr == "always":
		return true
	case expr == "not_empty":
		return strings.TrimSpace(output) != ""
	case strings.HasPrefix(expr, "contains:"):
		sub := strings.TrimPrefix(expr, "contains:")
		return strings.Contains(output, sub)
	case strings.HasPrefix(expr, "matches:"):
		pattern := strings.TrimPrefix(expr, "matches:")
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(output)
	case strings.HasPrefix(expr, "len > "):
		nStr := strings.TrimPrefix(expr, "len > ")
		n, err := strconv.Atoi(strings.TrimSpace(nStr))
		if err != nil {
			return false
		}
		return len(output) > n
	default:
		return false
	}
}

// toString coerces any value to string via fmt.Sprint.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
