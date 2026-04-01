package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/8op-org/gl1tch/internal/braincontext"
	"github.com/8op-org/gl1tch/internal/store"
)

// ExecutionContext is the shared state passed between pipeline steps.
// It provides concurrent-safe access to a nested map[string]any, addressable
// via dot-separated path expressions (e.g. "step.fetch.data.url").
type ExecutionContext struct {
	mu           sync.RWMutex
	data         map[string]any
	db           *store.Store
	injector     BrainInjector
	ragInjector  ragInjectorIface // optional RAG-based injector
	runID        int64
	workspaceCtx braincontext.WorkspaceContext
	stepOutputs  map[string]map[string]string // stepID → key → value
}

// ragInjectorIface is implemented by *brainrag.BrainInjector.
// Defined as an interface to avoid an import cycle in context.go.
type ragInjectorIface interface {
	InjectInto(ctx context.Context, prompt string) (string, error)
}

// ExecutionContextOption configures an ExecutionContext at construction time.
type ExecutionContextOption func(*ExecutionContext)

// WithStore attaches a result store to the ExecutionContext, making it
// available to db steps via ec.DB().
func WithStore(s *store.Store) ExecutionContextOption {
	return func(ec *ExecutionContext) { ec.db = s }
}

// NewExecutionContext returns an empty ExecutionContext with optional configuration.
func NewExecutionContext(opts ...ExecutionContextOption) *ExecutionContext {
	ec := &ExecutionContext{
		data:        make(map[string]any),
		stepOutputs: make(map[string]map[string]string),
	}
	for _, opt := range opts {
		opt(ec)
	}
	return ec
}

// DB returns the attached result store, or nil if none was configured.
func (c *ExecutionContext) DB() *store.Store { return c.db }

// SetRAGInjector attaches a RAG-based injector to the context.
func (c *ExecutionContext) SetRAGInjector(inj ragInjectorIface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ragInjector = inj
}

// GetRAGInjector returns the RAG injector, or nil if not set.
func (c *ExecutionContext) GetRAGInjector() ragInjectorIface {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ragInjector
}

// SetBrainInjector attaches a BrainInjector and the current run ID to the
// context. Called by the runner after ec creation when a brain injector is
// configured for the run.
func (c *ExecutionContext) SetBrainInjector(injector BrainInjector, runID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.injector = injector
	c.runID = runID
}

// GetBrainInjector returns the attached BrainInjector, or nil if none was set.
func (c *ExecutionContext) GetBrainInjector() BrainInjector {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.injector
}

// RunID returns the store run ID associated with this execution context.
// Returns 0 if the store was not configured or the run record was not created.
func (c *ExecutionContext) RunID() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runID
}

// SetStepOutput stores the value of a declared output key for a step.
// Thread-safe; used by the runner after each step completes.
func (c *ExecutionContext) SetStepOutput(stepID, key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stepOutputs == nil {
		c.stepOutputs = make(map[string]map[string]string)
	}
	if c.stepOutputs[stepID] == nil {
		c.stepOutputs[stepID] = make(map[string]string)
	}
	c.stepOutputs[stepID][key] = val
}

// StepOutput retrieves a declared output value for the given step and key.
// Returns ("", false) if the step or key has not been recorded yet.
func (c *ExecutionContext) StepOutput(stepID, key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if m, ok := c.stepOutputs[stepID]; ok {
		v, ok := m[key]
		return v, ok
	}
	return "", false
}

// SetWorkspaceContext attaches a WorkspaceContext to the execution context.
func (c *ExecutionContext) SetWorkspaceContext(wc braincontext.WorkspaceContext) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workspaceCtx = wc
}

// WorkspaceCtx returns the WorkspaceContext associated with this execution context.
func (c *ExecutionContext) WorkspaceCtx() braincontext.WorkspaceContext {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.workspaceCtx
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
