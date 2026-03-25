package pipeline

import (
	"fmt"
	"strings"
	"sync"
)

// ExecutionContext is the shared state passed between pipeline steps.
// It provides concurrent-safe access to a nested map[string]any, addressable
// via dot-separated path expressions (e.g. "step.fetch.data.url").
type ExecutionContext struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewExecutionContext returns an empty ExecutionContext.
func NewExecutionContext() *ExecutionContext {
	return &ExecutionContext{data: make(map[string]any)}
}

// Get retrieves the value at the dot-separated path. Returns (nil, false) if
// any segment of the path is missing or not a map.
func (c *ExecutionContext) Get(path string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return getPath(c.data, strings.Split(path, "."))
}

// Set stores value at the leaf of the dot-separated path, creating intermediate
// maps as needed.
func (c *ExecutionContext) Set(path string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	setPath(c.data, strings.Split(path, "."), value)
}

// Snapshot returns a shallow copy of the top-level data map. The returned map
// is safe to read without holding the lock; nested maps are shared references.
func (c *ExecutionContext) Snapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]any, len(c.data))
	for k, v := range c.data {
		out[k] = v
	}
	return out
}

// FlatStrings returns a flat map[string]string where each leaf value is
// coerced via fmt.Sprint. Useful for passing to plugin.Execute vars.
func (c *ExecutionContext) FlatStrings() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string)
	flattenStrings(c.data, "", out)
	return out
}

func flattenStrings(m map[string]any, prefix string, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			flattenStrings(nested, key, out)
		} else {
			out[key] = fmt.Sprint(v)
		}
	}
}

func getPath(m map[string]any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return nil, false
	}
	v, ok := m[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return v, true
	}
	nested, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	return getPath(nested, parts[1:])
}

func setPath(m map[string]any, parts []string, value any) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		m[parts[0]] = value
		return
	}
	nested, ok := m[parts[0]].(map[string]any)
	if !ok {
		nested = make(map[string]any)
		m[parts[0]] = nested
	}
	setPath(nested, parts[1:], value)
}
