package glitchd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogEntry is one captured slog record. We strip slog's structured
// attrs into a flat string so the desktop frontend doesn't have to
// know about slog internals.
type LogEntry struct {
	TimeMs  int64  `json:"time_ms"`
	Level   string `json:"level"`
	Source  string `json:"source"` // best-effort: "git", "claude", … or ""
	Message string `json:"message"`
	// Attrs is the slog key=value pairs joined into a single string,
	// e.g. "repo=gl1tch count=3". Kept compact for the UI; the full
	// raw line is also written to stderr by the inner handler.
	Attrs string `json:"attrs,omitempty"`
}

// LogBuffer is a fixed-size ring buffer of recent slog records. The
// brain popover reads from it to show the user "what are the
// collectors actually doing right now". Writes are O(1); reads return
// a flat copy newest-first.
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	cap     int
	next    int
	full    bool
}

// Logs is the singleton ring buffer the desktop reads from.
//
// 500 entries × ~150 bytes ≈ 75 KB resident — fine for an always-on
// surface, and it's enough to span ~30 minutes of normal collector
// chatter at the default intervals.
var Logs = NewLogBuffer(500)

// NewLogBuffer creates a ring buffer with the given capacity.
func NewLogBuffer(capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = 100
	}
	return &LogBuffer{
		entries: make([]LogEntry, capacity),
		cap:     capacity,
	}
}

// Push appends an entry, overwriting the oldest if the buffer is
// full. Safe for concurrent use.
func (b *LogBuffer) Push(e LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.next] = e
	b.next = (b.next + 1) % b.cap
	if b.next == 0 {
		b.full = true
	}
}

// Snapshot returns up to limit recent entries, newest first. limit ≤ 0
// returns everything currently buffered.
func (b *LogBuffer) Snapshot(limit int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	size := b.next
	if b.full {
		size = b.cap
	}
	if size == 0 {
		return nil
	}

	if limit <= 0 || limit > size {
		limit = size
	}

	out := make([]LogEntry, 0, limit)
	// Walk backwards from next-1 (most recent) wrapping around.
	idx := b.next - 1
	if idx < 0 {
		idx += b.cap
	}
	for range limit {
		out = append(out, b.entries[idx])
		idx--
		if idx < 0 {
			idx += b.cap
		}
	}
	return out
}

// teeHandler wraps an inner slog.Handler and also records every
// emitted record into Logs. This lets us preserve the existing
// stderr/file log output unchanged while making the records visible
// in the desktop UI.
type teeHandler struct {
	inner slog.Handler
}

// InstallLogTee replaces slog's default handler with one that writes
// every record both to the existing handler AND to the Logs ring
// buffer. Call once at process startup.
//
// Returns the previous default handler so callers can restore it on
// shutdown if they want, but the desktop binary doesn't bother — the
// process exits and the buffer goes with it.
func InstallLogTee() slog.Handler {
	prev := slog.Default().Handler()
	slog.SetDefault(slog.New(&teeHandler{inner: prev}))
	return prev
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	// Forward to the real handler first so stderr / file output isn't
	// delayed by ring-buffer bookkeeping.
	err := h.inner.Handle(ctx, r)

	// Capture into the ring buffer. We extract the source if any
	// attribute is named "collector", "source", or the message is
	// prefixed with "<name> collector:" / "<name>:" — covers both
	// the structured-attr style some collectors use and the
	// "<name> collector: <msg>" pattern most use today.
	entry := LogEntry{
		TimeMs:  r.Time.UnixMilli(),
		Level:   r.Level.String(),
		Message: r.Message,
	}
	var attrParts []string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "source" || a.Key == "collector" {
			if entry.Source == "" {
				entry.Source = a.Value.String()
			}
		}
		attrParts = append(attrParts, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
		return true
	})
	entry.Attrs = strings.Join(attrParts, " ")

	// Heuristic: pull "<name> collector:" prefix off the message into
	// the source field so the UI can color/filter by collector even
	// when the log site doesn't pass a structured attr.
	if entry.Source == "" {
		if i := strings.Index(r.Message, " collector:"); i > 0 {
			entry.Source = r.Message[:i]
			entry.Message = strings.TrimSpace(r.Message[i+len(" collector:"):])
		} else if i := strings.Index(r.Message, ":"); i > 0 && i < 30 {
			// e.g. "claude-projects: indexed session"
			candidate := r.Message[:i]
			if !strings.ContainsAny(candidate, " \t") {
				entry.Source = candidate
				entry.Message = strings.TrimSpace(r.Message[i+1:])
			}
		}
	}

	if r.Time.IsZero() {
		entry.TimeMs = time.Now().UnixMilli()
	}

	Logs.Push(entry)
	return err
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{inner: h.inner.WithGroup(name)}
}
