package themes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// activeThemeFile returns the path used to persist the active theme name.
// Uses os.UserConfigDir() + "/orcai/active_theme".
func activeThemeFile() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "orcai", "active_theme"), nil
}

// Registry holds all available theme bundles and tracks the active theme.
type Registry struct {
	bundles []Bundle
	byName  map[string]*Bundle
	active  *Bundle
}

// NewRegistry loads bundled themes and optional user themes from userDir,
// user themes win on name collision.  It also restores the previously active
// theme from the config persistence file.
func NewRegistry(userDir string) (*Registry, error) {
	bundled, err := LoadBundled()
	if err != nil {
		return nil, fmt.Errorf("themes: load bundled: %w", err)
	}

	var user []Bundle
	if userDir != "" {
		user, err = LoadUser(userDir)
		if err != nil {
			return nil, fmt.Errorf("themes: load user themes from %q: %w", userDir, err)
		}
	}

	r := &Registry{
		byName: make(map[string]*Bundle),
	}

	// Add bundled themes first.
	for i := range bundled {
		b := &bundled[i]
		r.bundles = append(r.bundles, *b)
	}
	// Merge user themes — user wins on collision.
	for _, u := range user {
		if idx, exists := r.indexByName(u.Name); exists {
			r.bundles[idx] = u
		} else {
			r.bundles = append(r.bundles, u)
		}
	}
	// Build index after merge.
	for i := range r.bundles {
		r.byName[r.bundles[i].Name] = &r.bundles[i]
	}

	// Default active: first bundled theme.
	if len(r.bundles) > 0 {
		r.active = &r.bundles[0]
	}

	// Restore persisted active theme if possible.
	if path, err := activeThemeFile(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			name := strings.TrimSpace(string(data))
			if b, ok := r.byName[name]; ok {
				r.active = b
			}
		}
	}

	return r, nil
}

// indexByName returns the slice index for the named theme, or -1 if not found.
func (r *Registry) indexByName(name string) (int, bool) {
	for i, b := range r.bundles {
		if b.Name == name {
			return i, true
		}
	}
	return -1, false
}

// All returns a copy of all registered theme bundles.
func (r *Registry) All() []Bundle {
	out := make([]Bundle, len(r.bundles))
	copy(out, r.bundles)
	return out
}

// Get looks up a bundle by name. Returns (bundle, true) if found.
func (r *Registry) Get(name string) (*Bundle, bool) {
	b, ok := r.byName[name]
	return b, ok
}

// Active returns the currently active theme bundle. It is never nil when the
// registry was created with at least one bundled theme.
func (r *Registry) Active() *Bundle {
	return r.active
}

// SetActive sets the active theme by name and persists the choice to disk.
// Returns an error if name is not found.
func (r *Registry) SetActive(name string) error {
	b, ok := r.byName[name]
	if !ok {
		return fmt.Errorf("themes: unknown theme %q", name)
	}
	r.active = b

	// Persist to config file.
	path, err := activeThemeFile()
	if err != nil {
		return fmt.Errorf("themes: resolve config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("themes: create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		return fmt.Errorf("themes: write active theme: %w", err)
	}
	return nil
}
