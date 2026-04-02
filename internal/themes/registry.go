package themes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// activeThemeFile returns the path used to persist the active theme name.
// Uses os.UserConfigDir() + "/glitch/active_theme".
func activeThemeFile() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "glitch", "active_theme"), nil
}

// Registry holds all available theme bundles and tracks the active theme.
type Registry struct {
	bundles     []Bundle
	byName      map[string]*Bundle
	active      *Bundle
	mu          sync.Mutex
	subscribers []chan<- string
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

	// Default active: prefer "nord", fall back to first available.
	if b, ok := r.byName["nord"]; ok {
		r.active = b
	} else if len(r.bundles) > 0 {
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

// BundlesByMode returns bundles matching the given mode ("dark" or "light").
func (r *Registry) BundlesByMode(mode string) []Bundle {
	var out []Bundle
	for _, b := range r.bundles {
		if b.Mode == mode {
			out = append(out, b)
		}
	}
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
// After updating the active theme it notifies all subscribers via non-blocking
// sends so a slow subscriber never blocks the caller.
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

	// Notify subscribers — non-blocking so slow listeners don't stall the caller.
	r.mu.Lock()
	subs := make([]chan<- string, len(r.subscribers))
	copy(subs, r.subscribers)
	r.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- name:
		default:
		}
	}

	return nil
}

// Subscribe registers ch to receive the new theme name whenever SetActive is
// called. Sends are non-blocking; if the channel is full the notification is
// dropped for that subscriber. ch must not be nil.
func (r *Registry) Subscribe(ch chan<- string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers = append(r.subscribers, ch)
}

// Unsubscribe removes ch from the subscriber list. It is safe to call even if
// ch was never subscribed.
func (r *Registry) Unsubscribe(ch chan<- string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.subscribers[:0]
	for _, s := range r.subscribers {
		if s != ch {
			out = append(out, s)
		}
	}
	r.subscribers = out
}

// SafeSubscribe creates a 1-element buffered channel, subscribes it, and
// returns it. The caller should call Unsubscribe when done.
func (r *Registry) SafeSubscribe() chan string {
	ch := make(chan string, 1)
	r.Subscribe(ch)
	return ch
}

// RefreshActive re-reads the active_theme config file and updates the active
// bundle pointer. This allows a long-running process (e.g. a standalone TUI)
// to pick up theme changes written by another process (e.g. the deck).
// Returns the name of the newly active theme, or an empty string when the
// file cannot be read or names an unknown theme.
func (r *Registry) RefreshActive() string {
	path, err := activeThemeFile()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(data))
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.byName[name]; ok {
		r.active = b
		return name
	}
	return ""
}
