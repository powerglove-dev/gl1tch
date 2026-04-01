package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
)

// stepInputPattern matches {{ steps.<id>.<key> }} template expressions.
var stepInputPattern = regexp.MustCompile(`\{\{\s*steps\.([^.}\s]+)\.([^}\s]+)\s*\}\}`)

// templateFuncs is the FuncMap available in all Interpolate calls.
// Functions follow Go template pipe conventions: the piped value is passed as
// the last argument, so {{.x | replace "old" "new"}} calls replace("old","new",x).
var templateFuncs = template.FuncMap{
	// default returns def when val is nil or the empty string.
	"default": func(def, val any) any {
		if val == nil {
			return def
		}
		if s, ok := val.(string); ok && s == "" {
			return def
		}
		return val
	},
	// env reads an environment variable.
	"env": os.Getenv,
	// get resolves a dot-separated path against a nested map[string]any.
	// Useful for step IDs that contain hyphens which are invalid in dot notation:
	//   {{get "step.ask-llama.data.value" .}}
	// {{ steps.X.Y }} patterns in the returned value are escaped so that
	// ResolveStepInputs does not attempt to resolve them — embedded content
	// (e.g. docs containing example YAML) should not be treated as templates.
	"get": func(path string, data map[string]any) any {
		v := getNestedPath(data, strings.Split(path, "."))
		if s, ok := v.(string); ok {
			return stepInputPattern.ReplaceAllStringFunc(s, func(m string) string {
				return strings.Replace(m, "{{", "{ {", 1)
			})
		}
		return v
	},
	"trim":        strings.TrimSpace,
	"upper":       strings.ToUpper,
	"lower":       strings.ToLower,
	"trimPrefix":  func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix":  func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
	"replace":     func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
	"split":       func(sep, s string) []string { return strings.Split(s, sep) },
	"join":        joinFunc,
	"contains":    func(substr, s string) bool { return strings.Contains(s, substr) },
	"hasPrefix":   func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
	"hasSuffix":   func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },
	"catLines":    func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " ") },
	"splitLines":  func(s string) []string { return strings.Split(s, "\n") },
	"toJson":      toJSONString,
	"fromJson":    fromJSONString,
}

func joinFunc(sep string, s any) string {
	switch v := s.(type) {
	case []string:
		return strings.Join(v, sep)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, sep)
	default:
		return fmt.Sprint(s)
	}
}

func toJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func fromJSONString(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

// getNestedPath walks a nested map[string]any using dot-path parts.
func getNestedPath(m map[string]any, parts []string) any {
	if len(parts) == 0 || m == nil {
		return nil
	}
	v, ok := m[parts[0]]
	if !ok {
		return nil
	}
	if len(parts) == 1 {
		return v
	}
	nested, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return getNestedPath(nested, parts[1:])
}

// protectStepInputs replaces {{ steps.x.y }} matches with __GLITCH_STEP_N__
// placeholders so the template engine doesn't evaluate them.
func protectStepInputs(s string) (string, []string) {
	var originals []string
	protected := stepInputPattern.ReplaceAllStringFunc(s, func(match string) string {
		placeholder := fmt.Sprintf("__GLITCH_STEP_%d__", len(originals))
		originals = append(originals, match)
		return placeholder
	})
	return protected, originals
}

// restoreStepInputs replaces __GLITCH_STEP_N__ placeholders with the originals.
func restoreStepInputs(s string, originals []string) string {
	for i, orig := range originals {
		s = strings.ReplaceAll(s, fmt.Sprintf("__GLITCH_STEP_%d__", i), orig)
	}
	return s
}

// Interpolate evaluates s as a Go text/template with vars as the data root.
//
// Use {{.key}} for direct field access and {{.a.b.c}} for nested maps.
// For step IDs containing hyphens, use the get function:
//
//	{{get "step.ask-llama.data.value" .}}
//
// If the template fails to parse or execute, the original string is returned
// unchanged. {{ steps.<id>.<key> }} patterns are preserved for ResolveStepInputs.
//
// Available functions: default, env, get, trim, upper, lower, trimPrefix, trimSuffix,
// replace, split, join, contains, hasPrefix, hasSuffix, catLines, splitLines,
// toJson, fromJson.
//
// Examples:
//
//	{{.param.query | upper | default "analyze this"}}
//	{{if .param.verbose}}Be detailed{{else}}Be concise{{end}}
//	{{env "HOME"}}
//	{{get "step.ask-llama.data.value" .}}
func Interpolate(s string, vars map[string]any) string {
	protected, stepInputs := protectStepInputs(s)

	tpl, err := template.New("").Option("missingkey=zero").Funcs(templateFuncs).Parse(protected)
	if err != nil {
		return s
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return s
	}
	result := strings.ReplaceAll(buf.String(), "<no value>", "")
	return restoreStepInputs(result, stepInputs)
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
