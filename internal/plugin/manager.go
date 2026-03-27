package plugin

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

// actionPlugin wraps a Plugin and injects _action into the vars map on Execute.
// It is returned by Manager.Get for category.action lookups.
type actionPlugin struct {
	Plugin
	action string
}

func (ap *actionPlugin) Name() string { return ap.Plugin.Name() + "." + ap.action }

func (ap *actionPlugin) Execute(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
	merged := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		merged[k] = v
	}
	merged["_action"] = ap.action
	return ap.Plugin.Execute(ctx, input, merged, w)
}

// TODO(translations): When a plugin execution context interface is added,
// inject a translations.Provider via a Translations() method so plugins can
// use the same operator-configured string overrides as the TUI. Wire it from
// translations.GlobalProvider() in the plugin host/executor setup.

// Manager holds the registry of all active plugins.
type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]Plugin
	categoryIndex map[string]Plugin // keyed by category prefix (e.g. "providers.claude")
}

// NewManager returns an empty plugin manager.
func NewManager() *Manager {
	return &Manager{
		plugins:       make(map[string]Plugin),
		categoryIndex: make(map[string]Plugin),
	}
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

// RegisterCategory registers a plugin under a category key and tracks the valid action.
// This enables hierarchical "category.action" lookups via Get.
// category is the prefix (e.g. "providers.claude"), action is the specific capability.
func (m *Manager) RegisterCategory(category, action string, p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Store under the category name. If multiple actions are registered, the last one wins —
	// the actual dispatch is done by injecting _action into the vars map.
	m.categoryIndex[category] = p
	// Also register under the full "category.action" name in the flat plugins map for direct lookup.
	fullName := category + "." + action
	if _, exists := m.plugins[fullName]; !exists {
		m.plugins[fullName] = &actionPlugin{Plugin: p, action: action}
	}
}

// Get returns a plugin by name.
// Resolution order:
//  1. Direct lookup in the flat plugins map.
//  2. Category lookup: split name on the last dot, look up category in categoryIndex,
//     return a wrapper that injects _action into the vars map.
func (m *Manager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Direct lookup.
	if p, ok := m.plugins[name]; ok {
		return p, true
	}

	// 2. Category lookup: split on last dot.
	idx := strings.LastIndex(name, ".")
	if idx <= 0 {
		return nil, false
	}
	category := name[:idx]
	action := name[idx+1:]
	if p, ok := m.categoryIndex[category]; ok {
		return &actionPlugin{Plugin: p, action: action}, true
	}

	return nil, false
}

// LoadWrappersFromDir scans dir for sidecar YAML files and registers all resulting plugins.
// Per-file errors (parse failures and duplicate names) are returned; they do not prevent other
// wrappers from being registered. If a sidecar has a category field, the adapter is also
// registered under that category via RegisterCategory.
func (m *Manager) LoadWrappersFromDir(dir string) []error {
	plugins, errs := LoadWrappers(dir)
	for _, p := range plugins {
		if err := m.Register(p); err != nil {
			errs = append(errs, err)
			continue
		}
		// If the adapter has a category, register it in the category index.
		if ca, ok := p.(*CliAdapter); ok && ca.Category() != "" {
			m.RegisterCategory(ca.Category(), ca.Name(), p)
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
