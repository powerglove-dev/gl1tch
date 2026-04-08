package capability

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the set of capabilities the runner knows about. It is the
// single source of truth that replaces the split between collector_registry
// and executor.Manager.
//
// Registration is name-keyed and idempotent-on-conflict (returns an error if
// the same name is registered twice). The assistant's capability picker reads
// from List/Names to build the menu it presents to the local model.
type Registry struct {
	mu sync.RWMutex
	by map[string]Capability
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{by: make(map[string]Capability)}
}

// Register adds a capability. Returns an error if the name is empty or
// already registered.
func (r *Registry) Register(c Capability) error {
	m := c.Manifest()
	if m.Name == "" {
		return fmt.Errorf("capability: register: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.by[m.Name]; exists {
		return fmt.Errorf("capability: register: %q already registered", m.Name)
	}
	r.by[m.Name] = c
	return nil
}

// Get looks up a capability by name.
func (r *Registry) Get(name string) (Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.by[name]
	return c, ok
}

// List returns every registered capability sorted by name. Stable order keeps
// the assistant's capability menu deterministic across runs.
func (r *Registry) List() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.by))
	for _, c := range r.by {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest().Name < out[j].Manifest().Name
	})
	return out
}

// Names returns the sorted list of registered capability names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.by))
	for n := range r.by {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
