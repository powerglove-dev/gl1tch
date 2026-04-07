package glitchd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
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

// teeHandler wraps an inner slog.Handler and ALSO records every
// emitted record into both:
//
//   1. the in-process Logs ring buffer (powers the brain popover's
//      live-tail view), and
//   2. the package-level shipQueue (an unbounded but capped slice
//      flushed to glitch-logs in ES on a 2s tick by the log
//      shipper goroutine started by InstallLogTee).
//
// This lets us preserve the existing stderr/file log output
// unchanged while making the records visible in the desktop UI AND
// queryable across restarts via Kibana. The two sinks are
// independent: ES outages don't affect the live ring buffer, and
// vice versa.
type teeHandler struct {
	inner slog.Handler
}

// shipQueue holds slog records waiting to be bulk-indexed into ES.
// Bounded so an extended ES outage doesn't hold the entire log
// history hostage in process memory; we drop oldest on overflow.
var (
	shipQueue   []map[string]any
	shipQueueMu sync.Mutex
	shipMaxLen  = 2000
	shipPID     = os.Getpid()
	shipHost, _ = os.Hostname()
)

// InstallLogTee replaces slog's default handler with one that writes
// every record to:
//
//   - the existing handler (stderr / file output),
//   - the in-process Logs ring buffer, and
//   - the ES log shipper queue (drained by a background goroutine
//     into glitch-logs every 2 seconds).
//
// The log shipper picks up an ES client from observer.yaml on
// startup. If ES is unreachable at install time the shipper still
// starts; it just retries on every flush tick and keeps the queue
// drained until ES comes back. ES outages cap the in-memory queue
// at shipMaxLen entries — older records get dropped, not held
// forever, so a long outage doesn't OOM the desktop.
//
// Returns the previous default handler so callers can restore it
// on shutdown if they want, but the desktop binary doesn't bother
// — the process exits and the buffer goes with it.
func InstallLogTee() slog.Handler {
	prev := slog.Default().Handler()
	slog.SetDefault(slog.New(&teeHandler{inner: prev}))
	go runLogShipper()
	return prev
}

// runLogShipper drains the shipQueue into glitch-logs on a 2s
// tick. Started exactly once by InstallLogTee. Lazy-resolves the
// ES client on every tick so a process that starts before ES is
// reachable still picks up the connection later — the shipper
// never gives up.
//
// Failures are logged via the inner handler (NOT slog.Default(),
// which would re-enter the tee and create a feedback loop) so the
// user has a stderr breadcrumb when ES is unreachable.
func runLogShipper() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		flushShipQueue()
	}
}

// flushShipQueue snapshots the current queue and bulk-indexes it.
// On any error the snapshotted records are NOT requeued — we
// accept the loss in exchange for not double-indexing on partial
// successes (the ES bulk API can succeed for some docs and fail
// for others, and the surface area for "exactly which ones?" is
// more trouble than it's worth for diagnostic logs).
func flushShipQueue() {
	shipQueueMu.Lock()
	if len(shipQueue) == 0 {
		shipQueueMu.Unlock()
		return
	}
	batch := shipQueue
	shipQueue = nil
	shipQueueMu.Unlock()

	cfg, err := collector.LoadConfig()
	if err != nil {
		return
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}
	client, err := esearch.New(addr)
	if err != nil {
		return
	}

	docs := make([]any, 0, len(batch))
	for _, d := range batch {
		docs = append(docs, d)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.BulkIndex(ctx, esearch.IndexLogs, docs)
}

// enqueueLogDoc is called from the teeHandler's Handle method
// after the record has been forwarded to the inner handler and
// pushed to the ring buffer. Drops the oldest entry when the
// queue exceeds shipMaxLen so a stuck shipper can't OOM us.
func enqueueLogDoc(doc map[string]any) {
	shipQueueMu.Lock()
	defer shipQueueMu.Unlock()
	if len(shipQueue) >= shipMaxLen {
		// Drop the oldest entry. A simple slice shift is O(N) but
		// only runs when the queue is full and the shipper is
		// stuck, so the cost is bounded by shipMaxLen and only
		// pays out during ES outages.
		shipQueue = shipQueue[1:]
	}
	shipQueue = append(shipQueue, doc)
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
	// attrsJSON keeps the structured key/value form so the ES doc
	// can carry the original shape (Kibana drills into nested keys
	// without re-parsing the joined string).
	attrsJSON := make(map[string]any)
	var attrParts []string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "source" || a.Key == "collector" {
			if entry.Source == "" {
				entry.Source = a.Value.String()
			}
		}
		attrParts = append(attrParts, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
		attrsJSON[a.Key] = a.Value.Any()
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

	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
		entry.TimeMs = ts.UnixMilli()
	}

	Logs.Push(entry)

	// Enqueue for ES. Doc shape mirrors logsMapping in
	// internal/esearch/mappings.go: timestamp + level + source +
	// message + attrs (joined string for the popover) + attrs_json
	// (structured for Kibana drill-downs) + process_pid + host_name
	// so multiple gl1tch processes are visually distinguishable.
	enqueueLogDoc(map[string]any{
		"timestamp":   ts.UTC().Format(time.RFC3339Nano),
		"level":       entry.Level,
		"source":      entry.Source,
		"message":     entry.Message,
		"attrs":       entry.Attrs,
		"attrs_json":  attrsJSON,
		"process_pid": shipPID,
		"host_name":   shipHost,
		"service":     "gl1tch",
	})
	return err
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{inner: h.inner.WithGroup(name)}
}
