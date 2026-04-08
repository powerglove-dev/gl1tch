package capability

import (
	"log/slog"
	"sync"
)

// EventSink is a per-process callback the desktop App registers to observe
// every document a capability successfully indexes. The desktop uses it to
// feed events into the deep-analysis queue without the capability package
// having to import pkg/glitchd. workspaceID is the owning workspace;
// source is the logical source label ("git", "github", "directory",
// "claude", …); docs is the slice of indexed objects.
//
// Sinks must NOT block — they are called from the capability tick
// goroutine and latency delays the next poll. The desktop implementation
// pushes onto a channel and returns immediately.
type EventSink func(workspaceID, source string, docs []any)

var (
	eventSinkMu sync.RWMutex
	eventSink   EventSink
)

// SetEventSink installs the process-wide event sink. Pass nil to clear.
// Safe to call before or after capabilities have started.
func SetEventSink(s EventSink) {
	eventSinkMu.Lock()
	eventSink = s
	eventSinkMu.Unlock()
}

// notifyEventSink fans indexed docs out to the registered sink, if any.
// Recovers from panics in the sink so a buggy desktop callback cannot take
// down the capability goroutine.
func notifyEventSink(workspaceID, source string, docs []any) {
	eventSinkMu.RLock()
	s := eventSink
	eventSinkMu.RUnlock()
	if s == nil || len(docs) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("event sink panicked",
				"workspace", workspaceID, "source", source, "panic", r)
		}
	}()
	s(workspaceID, source, docs)
}
