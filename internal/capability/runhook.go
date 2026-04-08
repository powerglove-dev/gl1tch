package capability

import (
	"sync"
	"time"
)

// RunReport is the per-poll heartbeat collectors emit so the brain
// (and the desktop UI) can show real "last attempted run" timestamps,
// not just "last indexed doc". This is what fills the gap between
// "ES has nothing for this collector yet" and "the collector is alive
// and trying, it just hasn't found anything to index".
type RunReport struct {
	// Name is the collector identifier (e.g. "git", "github").
	Name string
	// IndexedCount is the number of docs the run pushed to ES this
	// cycle. May be 0 for healthy "nothing new" polls.
	IndexedCount int
	// Duration is how long the cycle took (wall clock).
	Duration time.Duration
	// Err is non-nil if the cycle failed. The brain UI will surface
	// failed runs as warn alerts so the user notices stuck collectors.
	Err error
	// At is when the run completed.
	At time.Time
}

// RunRegistry holds the most recent RunReport for each collector. It's
// process-local — the desktop binary embeds the supervisor in-process
// so the registry is shared between the collector goroutines and the
// desktop UI without needing busd or any IPC.
type RunRegistry struct {
	mu   sync.RWMutex
	last map[string]RunReport
	// hook is an optional fan-out callback set by the supervisor.
	hook func(RunReport)
}

// Runs is the singleton registry. Collectors call Runs.Record after
// each poll cycle; consumers (the desktop's ListCollectors) call
// Runs.Snapshot to read the current state.
var Runs = &RunRegistry{last: map[string]RunReport{}}

// Record stores a RunReport, overwriting any prior entry for the same
// collector name. If a hook is installed (set by the supervisor for
// out-of-process publishing), it's called too.
func (r *RunRegistry) Record(rep RunReport) {
	if rep.Name == "" {
		return
	}
	if rep.At.IsZero() {
		rep.At = time.Now()
	}
	r.mu.Lock()
	r.last[rep.Name] = rep
	hook := r.hook
	r.mu.Unlock()
	if hook != nil {
		hook(rep)
	}
}

// Snapshot returns a copy of the registry keyed by collector name.
// Safe to read while collectors continue recording.
func (r *RunRegistry) Snapshot() map[string]RunReport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]RunReport, len(r.last))
	for k, v := range r.last {
		out[k] = v
	}
	return out
}

// SetHook installs an optional fan-out callback fired on each Record.
// The supervisor uses it to publish collector heartbeats onto busd so
// glitch-notify (and any future subscriber) can react. Pass nil to
// clear it.
func (r *RunRegistry) SetHook(fn func(RunReport)) {
	r.mu.Lock()
	r.hook = fn
	r.mu.Unlock()
}

// RecordRun is a small convenience for collectors: pass the start time
// and the result of a single poll cycle and it does the bookkeeping
// (computing duration, calling Record). Keeps the call sites in each
// collector down to a single line.
func RecordRun(name string, start time.Time, indexed int, err error) {
	Runs.Record(RunReport{
		Name:         name,
		IndexedCount: indexed,
		Duration:     time.Since(start),
		Err:          err,
		At:           time.Now(),
	})
}
