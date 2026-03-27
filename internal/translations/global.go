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
// Call this once during startup (e.g. in switchboard model creation).
// Subsequent calls to GlobalProvider() will return p.
func SetGlobalProvider(p Provider) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.p = p
}
