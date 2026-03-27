package themes

import "sync"

// global holds the process-level singleton registry.
var global struct {
	once sync.Once
	mu   sync.Mutex
	r    *Registry
}

// GlobalRegistry returns the process-level singleton *Registry.
// It returns nil if SetGlobalRegistry has not been called yet — callers must
// handle a nil return value gracefully.
func GlobalRegistry() *Registry {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.r
}

// SetGlobalRegistry stores r as the process-level singleton registry.
// Call this once after loading themes (e.g. from main or switchboard).
// Subsequent calls to GlobalRegistry() will return r.
func SetGlobalRegistry(r *Registry) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.r = r
}
