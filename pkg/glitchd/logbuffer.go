package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// LogEntry is one captured slog record.
type LogEntry struct {
	TimeMs  int64  `json:"time_ms"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
	Attrs   string `json:"attrs,omitempty"`
}

// QueryRecentLogs fetches the most recent log entries from the
// glitch-logs Elasticsearch index, newest first.
func QueryRecentLogs(es *esearch.Client, limit int) ([]LogEntry, error) {
	if es == nil {
		return []LogEntry{}, nil
	}
	if limit <= 0 {
		limit = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := es.Search(ctx, []string{esearch.IndexLogs}, map[string]any{
		"size":  limit,
		"sort":  []map[string]any{{"timestamp": map[string]any{"order": "desc"}}},
		"query": map[string]any{"match_all": map[string]any{}},
	})
	if err != nil {
		return nil, err
	}

	out := make([]LogEntry, 0, len(resp.Results))
	for _, r := range resp.Results {
		var doc struct {
			Timestamp string `json:"timestamp"`
			Level     string `json:"level"`
			Source    string `json:"source"`
			Message   string `json:"message"`
			Attrs     string `json:"attrs"`
		}
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339Nano, doc.Timestamp)
		out = append(out, LogEntry{
			TimeMs:  t.UnixMilli(),
			Level:   doc.Level,
			Source:  doc.Source,
			Message: doc.Message,
			Attrs:   doc.Attrs,
		})
	}
	return out, nil
}

// esTeeHandler wraps a real slog.Handler (a TextHandler bound to
// stderr) and enqueues every record into shipQueue for the background
// shipper to bulk-index into glitch-logs.
//
// Critical: the inner MUST be a "real" handler that writes directly
// to its own io.Writer (TextHandler/JSONHandler). It MUST NOT be the
// slog package's defaultHandler — that one writes via log.Output,
// which holds the log package's mutex while writing. Combined with
// slog.SetDefault's side effect of rewiring log.SetOutput through the
// new default Handler (which is us), the chain becomes:
//
//	teeHandler.Handle → defaultHandler.Handle → log.Output (locks log.mu)
//	  → handlerWriter.Write → teeHandler.Handle (re-entry)
//	  → defaultHandler.Handle → log.Output (TRIES to re-lock log.mu)
//	  → DEADLOCK
//
// That is exactly the bug that made glitch-logs ship 0 records: the
// very first slog.Info call after InstallLogTee deadlocked the
// calling goroutine on the log package mutex, so enqueueLogDoc was
// never reached. Wrapping a real TextHandler eliminates the recursion
// because TextHandler writes its bytes directly to os.Stderr without
// touching the log package.
type esTeeHandler struct {
	inner slog.Handler
}

var (
	shipQueue   []map[string]any
	shipQueueMu sync.Mutex
	shipMaxLen  = 2000
	shipPID     = os.Getpid()
	shipHost, _ = os.Hostname()
)

// InstallLogTee replaces slog's default handler with one that writes
// to stderr (via a fresh TextHandler) AND enqueues every record for
// shipping to ES. Safe to call before InitPodManager — the shipper
// requeues until EsClient() is non-nil.
//
// We deliberately discard slog.Default().Handler() instead of wrapping
// it. The default handler is Go's defaultHandler which writes through
// log.Output and would deadlock under us — see esTeeHandler's doc
// comment for the gory chain.
//
// Level defaults to DEBUG so collector verification logs
// ("git collector: poll tick", "directory collector: scanning") reach
// both stderr and glitch-logs. Override with GL1TCH_LOG_LEVEL=info if
// the volume ever becomes a problem — logs are cheap and the whole
// point of this tee is that "did the collector actually run for my
// workspace" is a grep away instead of a stderr screenshot round trip.
func InstallLogTee() {
	level := slog.LevelDebug
	switch strings.ToLower(os.Getenv("GL1TCH_LOG_LEVEL")) {
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(&esTeeHandler{inner: inner}))
	go runLogShipper()
}

func runLogShipper() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		flushShipQueue()
	}
}

func flushShipQueue() {
	shipQueueMu.Lock()
	if len(shipQueue) == 0 {
		shipQueueMu.Unlock()
		return
	}
	batch := shipQueue
	shipQueue = nil
	shipQueueMu.Unlock()

	es := EsClient()
	if es == nil {
		// ES not ready yet; put records back and retry next tick.
		shipQueueMu.Lock()
		shipQueue = append(batch, shipQueue...)
		shipQueueMu.Unlock()
		return
	}

	docs := make([]any, len(batch))
	for i, d := range batch {
		docs[i] = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := es.BulkIndex(ctx, esearch.IndexLogs, docs); err != nil {
		// Write directly to stderr — NOT via slog — so a transient ES
		// outage doesn't recurse back through us.
		fmt.Fprintf(os.Stderr, "glitch: log-shipper: BulkIndex error: %v\n", err)
	}
}

func enqueueLogDoc(doc map[string]any) {
	shipQueueMu.Lock()
	defer shipQueueMu.Unlock()
	if len(shipQueue) >= shipMaxLen {
		shipQueue = shipQueue[1:] // drop oldest on overflow
	}
	shipQueue = append(shipQueue, doc)
}

func (h *esTeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *esTeeHandler) Handle(ctx context.Context, r slog.Record) error {
	// Inner is a TextHandler that writes directly to stderr — no log
	// package involvement, so no re-entry through us.
	err := h.inner.Handle(ctx, r)

	entry := LogEntry{
		TimeMs:  r.Time.UnixMilli(),
		Level:   r.Level.String(),
		Message: r.Message,
	}

	attrsJSON := make(map[string]any)
	var attrParts []string
	r.Attrs(func(a slog.Attr) bool {
		if (a.Key == "source" || a.Key == "collector") && entry.Source == "" {
			entry.Source = a.Value.String()
		}
		attrParts = append(attrParts, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
		attrsJSON[a.Key] = a.Value.Any()
		return true
	})
	entry.Attrs = strings.Join(attrParts, " ")

	// Best-effort source extraction from message prefix (e.g. "git: ...")
	if entry.Source == "" {
		if i := strings.Index(r.Message, " collector:"); i > 0 {
			entry.Source = r.Message[:i]
			entry.Message = strings.TrimSpace(r.Message[i+len(" collector:"):])
		} else if i := strings.Index(r.Message, ":"); i > 0 && i < 30 {
			if candidate := r.Message[:i]; !strings.ContainsAny(candidate, " \t") {
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

	// Elastic APM: every level-ERROR (and above) record is also
	// forwarded to the APM error sink so it lands in Kibana's APM
	// Errors UI with grouping + trace correlation. No-op when the
	// sink is nil (APM disabled, apm-server unreachable at startup).
	// Plain slog records don't carry a Go stack, so the APM doc
	// we build here has no stacktrace — call telemetry.CaptureError
	// explicitly from recover() blocks if you want the frames.
	if r.Level >= slog.LevelError {
		telemetry.CaptureLog(ctx, r.Message, attrsJSON)
	}

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

func (h *esTeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &esTeeHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *esTeeHandler) WithGroup(name string) slog.Handler {
	return &esTeeHandler{inner: h.inner.WithGroup(name)}
}
