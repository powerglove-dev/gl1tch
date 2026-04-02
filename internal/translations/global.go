package translations

import "sync"

// global holds the process-level singleton provider.
var global struct {
	mu sync.Mutex
	p  Provider
}

// GlobalProvider returns the process-level singleton Provider.
// It returns nil if SetGlobalProvider has not been called yet — callers must
// handle a nil return value gracefully (use the Safe helper or a nil check).
func GlobalProvider() Provider {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.p
}

// SetGlobalProvider stores p as the process-level singleton provider.
// Call this once during startup (e.g. in deck model creation).
// Subsequent calls to GlobalProvider() will return p.
func SetGlobalProvider(p Provider) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.p = p
}

// RebuildChain constructs the standard three-layer provider chain and sets it
// as the global provider. Call this at startup and whenever the active theme
// changes (passing the new theme's Strings map).
//
// Priority: user ~/.config/glitch/translations.yaml > themeStrings > defaults.
func RebuildChain(themeStrings map[string]string) {
	SetGlobalProvider(NewChain(
		NewYAMLProvider(),
		NewMapProvider(themeStrings),
		NewDefaultProvider(),
	))
}
