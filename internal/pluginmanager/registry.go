package pluginmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RegistryFileName is the name of the installed-plugins state file.
const RegistryFileName = "plugins.yaml"

// InstalledPlugin records a single installed plugin entry.
type InstalledPlugin struct {
	Name        string    `yaml:"name"`
	Source      string    `yaml:"source"`       // "owner/repo"
	Version     string    `yaml:"version"`
	BinaryPath  string    `yaml:"binary_path"`
	SidecarPath string    `yaml:"sidecar_path"`
	InstalledAt time.Time `yaml:"installed_at"`
}

// Registry tracks all installed plugins in a YAML state file.
type Registry struct {
	path    string
	Plugins []InstalledPlugin `yaml:"plugins"`
}

// LoadRegistry reads the registry from dir/plugins.yaml.
// If the file does not exist an empty registry is returned (not an error).
func LoadRegistry(dir string) (*Registry, error) {
	path := filepath.Join(dir, RegistryFileName)
	r := &Registry{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load plugin registry: %w", err)
	}
	if err := yaml.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse plugin registry: %w", err)
	}
	return r, nil
}

// Save writes the registry back to disk.
func (r *Registry) Save() error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal plugin registry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("save plugin registry: mkdir: %w", err)
	}
	return os.WriteFile(r.path, data, 0o644)
}

// Add adds or replaces a plugin entry.
func (r *Registry) Add(p InstalledPlugin) {
	for i, existing := range r.Plugins {
		if existing.Name == p.Name {
			r.Plugins[i] = p
			return
		}
	}
	r.Plugins = append(r.Plugins, p)
}

// Remove deletes a plugin entry by name. Returns false if not found.
func (r *Registry) Remove(name string) bool {
	for i, p := range r.Plugins {
		if p.Name == name {
			r.Plugins = append(r.Plugins[:i], r.Plugins[i+1:]...)
			return true
		}
	}
	return false
}

// Get returns the installed plugin entry for name, or false if not found.
func (r *Registry) Get(name string) (InstalledPlugin, bool) {
	for _, p := range r.Plugins {
		if p.Name == name {
			return p, true
		}
	}
	return InstalledPlugin{}, false
}
