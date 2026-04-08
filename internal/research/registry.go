package research

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrDuplicateResearcher is returned by Registry.Register when a researcher
// with the same Name is already registered.
var ErrDuplicateResearcher = errors.New("research: duplicate researcher name")

// Registry is the process-wide registry of Researchers the loop can pick
// from. It is safe for concurrent reads and writes; in practice, registration
// happens once at startup and Lookup is called from the gather stage.
type Registry struct {
	mu          sync.RWMutex
	researchers map[string]Researcher
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{researchers: make(map[string]Researcher)}
}

// Register adds a researcher to the registry. Returns ErrDuplicateResearcher
// (wrapped with the offending name) if a researcher with the same Name is
// already registered.
func (r *Registry) Register(researcher Researcher) error {
	if researcher == nil {
		return errors.New("research: cannot register nil researcher")
	}
	name := researcher.Name()
	if name == "" {
		return errors.New("research: researcher Name() must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.researchers[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateResearcher, name)
	}
	r.researchers[name] = researcher
	return nil
}

// Lookup returns the researcher registered under name. The second return is
// false if no researcher is registered under that name; it never panics.
func (r *Registry) Lookup(name string) (Researcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	researcher, ok := r.researchers[name]
	return researcher, ok
}

// List returns all registered researchers sorted by Name. Used by the
// planner to build the (Name, Describe) menu shown to the local model.
func (r *Registry) List() []Researcher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Researcher, 0, len(r.researchers))
	for _, researcher := range r.researchers {
		out = append(out, researcher)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names returns the registered researcher names sorted alphabetically.
// Convenience for tests and for `glitch researcher list`.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.researchers))
	for name := range r.researchers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
