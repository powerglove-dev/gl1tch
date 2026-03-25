package plugin

import (
	"fmt"
	"sync"
)

// Manager holds the registry of all active plugins.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewManager returns an empty plugin manager.
func NewManager() *Manager {
	return &Manager{plugins: make(map[string]Plugin)}
}

// Register adds a plugin. Returns an error if a plugin with the same name is already registered.
func (m *Manager) Register(p Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin %q already registered", p.Name())
	}
	m.plugins[p.Name()] = p
	return nil
}

// Get returns a plugin by name.
func (m *Manager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	return p, ok
}

// LoadWrappersFromDir scans dir for sidecar YAML files and registers all resulting plugins.
// Per-file errors (parse failures and duplicate names) are returned; they do not prevent other
// wrappers from being registered.
func (m *Manager) LoadWrappersFromDir(dir string) []error {
	plugins, errs := LoadWrappers(dir)
	for _, p := range plugins {
		if err := m.Register(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// List returns all registered plugins in no guaranteed order.
func (m *Manager) List() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, p)
	}
	return out
}
