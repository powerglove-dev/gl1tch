// apm_error.go ships gl1tch's Go errors and recovered panics to the
// Elastic APM Server's intake endpoint so they land in the Kibana APM
// "Errors" UI (grouped, stack-traced, trace-correlated) on top of the
// existing glitch-logs text rows.
//
// Why a handwritten NDJSON sink instead of the go.elastic.co/apm Go
// agent? Because the OTel SDK is already doing the heavy lifting for
// tracing and metrics, and pulling in the Elastic APM Go agent would
// duplicate the instrumentation stack: two tracer providers, two
// span models, two sets of context propagators. The APM intake
// schema for errors is small and stable — ~70 lines of doc shape —
// and posting raw NDJSON to /intake/v2/events gives us the APM UI
// without the dependency footprint.
//
// Schema reference:
//
//	https://www.elastic.co/guide/en/apm/guide/current/api-events.html
//	https://github.com/elastic/apm-server/blob/main/docs/spec/v2/error.json
//
// One metadata event + one error event per POST. Metadata carries the
// service name/version so apm-server groups errors by service in the
// UI. The error event carries culprit, exception, log, stacktrace,
// labels, and the trace/transaction IDs pulled from the caller's
// context so Kibana can navigate from an error back to the span that
// owned it.
package telemetry

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// apmIntakeTimeout caps every POST to /intake/v2/events. Five seconds
// is the same ceiling the OTel BatchSpanProcessor uses for its ES
// exporter fallback — consistent "ES-family service is wedged, drop
// this batch and move on" semantics across the telemetry package.
const apmIntakeTimeout = 5 * time.Second

// apmErrorSink is the package-level singleton that buffers error
// documents and POSTs them to apm-server. Nil-safe: the Capture path
// checks the sink pointer before every send so callers don't have to
// know whether APM was successfully wired at Setup time.
type apmErrorSink struct {
	endpoint string // e.g. "http://localhost:8200/intake/v2/events"
	client   *http.Client
	service  apmService

	// lastFailureLogMu guards lastFailureLogAt so the per-POST rate
	// limit on "apm: intake POST failed" stderr noise doesn't itself
	// become a performance problem under a prolonged apm-server
	// outage. We log at most once every 30s.
	lastFailureLogMu sync.Mutex
	lastFailureLogAt time.Time
}

// apmService is the metadata-event service block. Sent with every
// intake POST so apm-server groups docs by (service.name,
// service.version) in the APM UI.
type apmService struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Agent   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agent"`
	Language struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"language"`
}

var (
	// activeSink is the live APM sink. Atomic swap so Setup can be
	// called multiple times in tests without data races. Consumers
	// (pkg/glitchd/logbuffer.go, internal/telemetry/CaptureError) use
	// ActiveErrorSink() to fetch a snapshot for their current call.
	activeSink atomic.Pointer[apmErrorSink]
)

// newAPMErrorSink constructs a sink pointed at apm-server's intake
// endpoint for the given service. endpoint is the base URL (scheme +
// host + port) of apm-server — the "/intake/v2/events" path is
// appended here so callers pass the same value as the OTLP exporter.
func newAPMErrorSink(endpoint, serviceName string) *apmErrorSink {
	s := apmService{
		Name:    serviceName,
		Version: "dev",
	}
	// Kibana's APM UI groups errors by (service, agent.name). Tagging
	// ourselves "gl1tch/otel" instead of "otlp" makes the UI separate
	// gl1tch-emitted errors from any other OTel-instrumented service
	// that happens to share the apm-server.
	s.Agent.Name = "gl1tch"
	s.Agent.Version = "dev"
	s.Language.Name = "go"
	s.Language.Version = runtime.Version()

	return &apmErrorSink{
		endpoint: strings.TrimRight(endpoint, "/") + "/intake/v2/events",
		client: &http.Client{
			Timeout: apmIntakeTimeout,
		},
		service: s,
	}
}

// installAPMErrorSink swaps in a new sink as the active one. Called
// from telemetry.Setup after the OTLP exporter is wired. Passing nil
// is valid and disables APM error shipping for any subsequent
// Capture / slog.Error call — used by the SetupWithoutAPM path in
// tests.
func installAPMErrorSink(s *apmErrorSink) {
	activeSink.Store(s)
}

// ActiveErrorSink returns the current APM sink, or nil when APM was
// never wired (no endpoint set or Setup failed). Consumers that want
// to piggyback on the sink (pkg/glitchd/logbuffer.go routes
// slog.LevelError records here) should call this on every emit
// rather than caching the pointer, so a mid-run Setup-swap is
// respected.
func ActiveErrorSink() ErrorSink {
	s := activeSink.Load()
	if s == nil {
		return nil
	}
	return s
}

// ErrorSink is the minimum surface pkg/glitchd/logbuffer.go needs
// from this package without importing the full apmErrorSink type.
// Keeping the interface in the telemetry package means the logbuffer
// ↔ telemetry dependency stays one-way.
type ErrorSink interface {
	// CaptureLog forwards a level-ERROR slog record to APM. No Go
	// stack is attached — plain slog records don't carry one. Attrs
	// are flattened into labels for the APM UI's filter controls.
	CaptureLog(ctx context.Context, message string, attrs map[string]any)
	// CaptureError forwards a recovered panic or an explicit error
	// with a Go stack trace snapped from runtime.Callers at call
	// time. stackSkip controls how many frames runtime.Callers
	// skips; callers that wrap Capture themselves should add 1.
	CaptureError(ctx context.Context, err error, attrs map[string]any, stackSkip int)
}

// CaptureError is the package-level convenience wrapper that hides
// the ActiveErrorSink() nil check from call sites. Every `recover()`
// block in gl1tch should funnel through here so a missing apm-server
// degrades to a no-op instead of a nil deref.
//
// stackSkip: 0 captures the stack starting at the frame that called
// CaptureError. If you're calling this from inside another helper,
// pass 1.
func CaptureError(ctx context.Context, err error, attrs map[string]any, stackSkip int) {
	if err == nil {
		return
	}
	s := ActiveErrorSink()
	if s == nil {
		return
	}
	// +1 skips this CaptureError frame itself so the APM stack starts
	// at the caller (the recover block, not telemetry internals).
	s.CaptureError(ctx, err, attrs, stackSkip+1)
}

// CaptureLog is the package-level wrapper for the slog.Error path.
// Callers flatten slog attrs into a map and hand us the message; we
// stamp trace/transaction IDs from ctx and forward to the sink.
func CaptureLog(ctx context.Context, message string, attrs map[string]any) {
	s := ActiveErrorSink()
	if s == nil {
		return
	}
	s.CaptureLog(ctx, message, attrs)
}

// ── apmErrorSink implementation ────────────────────────────────────

// CaptureLog ships a plain slog.Error record to APM as an error doc
// with no stacktrace. Kibana's APM Errors UI groups these by culprit
// (the message, truncated) + service, so a recurring "git collector:
// poll error" lights up the group count instead of burying each
// instance.
func (s *apmErrorSink) CaptureLog(ctx context.Context, message string, attrs map[string]any) {
	doc := s.buildErrorDoc(ctx, message, message, nil, attrs)
	s.send(doc)
}

// CaptureError ships a Go error + stack trace. Used by the recover()
// paths in internal/collector/pod.go, internal/pipeline/runner.go,
// internal/brain/service.go, and anywhere else a panic gets caught.
func (s *apmErrorSink) CaptureError(ctx context.Context, err error, attrs map[string]any, stackSkip int) {
	msg := err.Error()
	// +1 skips this CaptureError method frame so the top of the stack
	// is the caller of telemetry.CaptureError.
	frames := captureStack(stackSkip + 1)
	doc := s.buildErrorDoc(ctx, msg, msg, frames, attrs)
	s.send(doc)
}

// buildErrorDoc assembles the APM intake error event. culprit appears
// prominently in the Errors UI as the group heading — we use the full
// message (truncated) because gl1tch's slog.Error messages are
// already short, descriptive, and stable across runs, which is
// exactly what grouping wants.
func (s *apmErrorSink) buildErrorDoc(
	ctx context.Context,
	culprit string,
	message string,
	stackframes []apmStackFrame,
	attrs map[string]any,
) map[string]any {
	// Span context pull for trace correlation. When a recover() fires
	// inside a collector goroutine the active span is
	// collector.run — Kibana's APM UI turns the error event's
	// trace.id + transaction.id into a clickable link back to that
	// span, which is exactly the "show me the trace for this crash"
	// affordance we've been missing.
	var traceID, txnID string
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		traceID = sc.TraceID().String()
		txnID = sc.SpanID().String()
	}

	now := time.Now().UTC()
	// APM's timestamp field is microseconds since Unix epoch. NOT
	// nanoseconds — the schema is explicit about this and the intake
	// rejects values that look like they're in ns.
	timestampUs := now.UnixNano() / 1000

	// grouping_key is a sha1 of (service.name + culprit + top frame)
	// so repeated errors from the same call site collapse into one
	// row in the Errors UI. apm-server will compute its own grouping
	// key if we omit ours, but controlling it from here means our
	// groups follow the dimensions we care about (service +
	// culprit), not whatever default apm-server picks.
	h := sha1.New()
	h.Write([]byte(s.service.Name))
	h.Write([]byte{0})
	h.Write([]byte(culprit))
	if len(stackframes) > 0 {
		h.Write([]byte{0})
		h.Write([]byte(stackframes[0].Filename))
		h.Write([]byte{0})
		fmt.Fprintf(h, "%d", stackframes[0].LineNo)
	}
	groupingKey := hex.EncodeToString(h.Sum(nil))

	// Error ID — 16 random bytes per APM spec. Use crypto-grade
	// randomness via trace IDs if we have them, otherwise fall back
	// to timestamp + a counter. APM uses this as the document ID in
	// the error index.
	var errID string
	if traceID != "" {
		// Reuse the first 16 chars of the trace ID for readability
		// in Kibana. Two errors inside the same trace will still
		// have different doc IDs because apm-server re-hashes.
		errID = traceID[:16] + strings.ReplaceAll(now.Format("150405.000"), ".", "")
		if len(errID) > 32 {
			errID = errID[:32]
		}
	} else {
		errID = fmt.Sprintf("%032x", now.UnixNano())[:32]
	}

	excType := "error"
	if culprit != "" {
		// The first colon-delimited prefix is usually the collector
		// or subsystem name (e.g. "git collector: poll error"); use
		// that as the exception.type so the Errors UI groups by
		// subsystem, matching our log shape.
		if i := strings.Index(culprit, ":"); i > 0 && i < 40 {
			excType = strings.TrimSpace(culprit[:i])
		}
	}

	exc := map[string]any{
		"type":    excType,
		"message": message,
		"handled": true, // we recovered; unhandled=false would look like a crash
	}
	if len(stackframes) > 0 {
		exc["stacktrace"] = stackframes
	}

	errorEvent := map[string]any{
		"id":           errID,
		"timestamp":    timestampUs,
		"culprit":      culprit,
		"grouping_key": groupingKey,
		"exception":    exc,
		"log": map[string]any{
			"level":       "error",
			"message":     message,
			"logger_name": "gl1tch",
		},
	}
	if traceID != "" {
		errorEvent["trace_id"] = traceID
	}
	if txnID != "" {
		errorEvent["parent_id"] = txnID
		errorEvent["transaction_id"] = txnID
	}
	if len(attrs) > 0 {
		// Labels must be primitives (string/number/bool) per the APM
		// spec. Coerce everything via fmt.Sprint so the intake doesn't
		// reject the whole batch over a []string attribute.
		labels := make(map[string]any, len(attrs))
		for k, v := range attrs {
			switch v := v.(type) {
			case string, bool, int, int32, int64, float32, float64:
				labels[k] = v
			default:
				labels[k] = fmt.Sprint(v)
			}
		}
		errorEvent["context"] = map[string]any{"tags": labels}
	}

	return map[string]any{"error": errorEvent}
}

// send posts one error doc to apm-server's intake endpoint. The body
// is two NDJSON lines: a metadata event followed by the error event.
// apm-server accepts batches of multiple error events under a single
// metadata header, but gl1tch's error rate is low enough that
// batching buys nothing — shipping one-per-POST simplifies failure
// semantics (drop-on-error, no partial-batch accounting).
func (s *apmErrorSink) send(doc map[string]any) {
	var buf bytes.Buffer

	// Line 1: metadata event. apm-server requires this as the first
	// event in every stream; it carries service, process, and system
	// fields that attach to every subsequent event in the request.
	metadata := map[string]any{
		"metadata": map[string]any{
			"service": s.service,
			"process": map[string]any{
				"pid": os.Getpid(),
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(metadata); err != nil {
		s.logFailure("encode metadata", err)
		return
	}

	// Line 2: the error event itself.
	if err := json.NewEncoder(&buf).Encode(doc); err != nil {
		s.logFailure("encode error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), apmIntakeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, &buf)
	if err != nil {
		s.logFailure("build request", err)
		return
	}
	// application/x-ndjson is the MIME the intake endpoint explicitly
	// expects; sending application/json returns 400.
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logFailure("POST", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		s.logFailure(fmt.Sprintf("POST status %d", resp.StatusCode), nil)
	}
}

// logFailure writes a rate-limited error line directly to stderr. NOT
// via slog — slog.Error would route back through the tee handler
// which would attempt another APM POST, which would fail again, and
// we'd have an unbounded error loop. Direct stderr is the only safe
// escape hatch from the telemetry package.
//
// We rate-limit to one line per 30s so a prolonged apm-server outage
// shows up once, not on every error.
func (s *apmErrorSink) logFailure(op string, err error) {
	s.lastFailureLogMu.Lock()
	defer s.lastFailureLogMu.Unlock()
	if time.Since(s.lastFailureLogAt) < 30*time.Second {
		return
	}
	s.lastFailureLogAt = time.Now()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gl1tch: apm intake %s failed: %v\n", op, err)
	} else {
		fmt.Fprintf(os.Stderr, "gl1tch: apm intake %s failed\n", op)
	}
}

// ── stack capture ──────────────────────────────────────────────────

// apmStackFrame matches APM's spec/v2/error.json frame shape. Only
// filename, lineno, and function are required; abs_path and module
// are populated too because Kibana's stack viewer falls back to them
// when filename alone is ambiguous (vendored packages, generated code).
type apmStackFrame struct {
	Filename string `json:"filename"`
	LineNo   int    `json:"lineno"`
	Function string `json:"function"`
	AbsPath  string `json:"abs_path,omitempty"`
	Module   string `json:"module,omitempty"`
}

// captureStack snaps a Go call stack via runtime.Callers and maps it
// into APM's frame shape. skip is the number of frames to trim from
// the top — 0 means "start at the frame that called captureStack".
//
// We cap at 32 frames because Kibana's stack viewer truncates after
// the first 20 anyway and our deepest real stack (brain → pipeline
// runner → provider adapter) is nowhere near 32.
func captureStack(skip int) []apmStackFrame {
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	// +2 skips runtime.Callers and captureStack itself; the caller's
	// `skip` parameter lets the recover block skip its own frames on
	// top of that.
	n := runtime.Callers(2+skip, pcs)
	if n == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs[:n])
	out := make([]apmStackFrame, 0, n)
	for {
		f, more := frames.Next()
		// Skip runtime goroutine-exit frames — they add noise to the
		// top of every captured stack without telling us anything
		// about gl1tch's code path.
		if strings.HasPrefix(f.Function, "runtime.") {
			if !more {
				break
			}
			continue
		}
		file := f.File
		module := ""
		if lastSep := strings.LastIndex(f.Function, "/"); lastSep >= 0 {
			module = f.Function[:lastSep]
		}
		out = append(out, apmStackFrame{
			Filename: filepath.Base(file),
			LineNo:   f.Line,
			Function: f.Function,
			AbsPath:  file,
			Module:   module,
		})
		if !more {
			break
		}
	}
	return out
}
