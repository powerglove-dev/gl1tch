## 1. Rename switchboard → deck

- [x] 1.1 Rename `internal/console/switchboard.go` to `internal/console/deck.go`
- [x] 1.2 Rename `internal/console/switchboard_test.go` to `internal/console/deck_test.go`
- [x] 1.3 Replace all identifier references to `switchboard`/`Switchboard` with `deck`/`Deck` inside `internal/console/deck.go` and `deck_test.go`
- [x] 1.4 Update all other files in `internal/console/` that reference switchboard identifiers (panel files, signal_board.go, pipeline_bus.go, etc.)
- [x] 1.5 Update callers in `cmd/` that reference the console package switchboard identifiers
- [x] 1.6 Run `go build ./...` and `go vet ./...` — zero errors gate

## 2. Redirect OTel traces to file

- [x] 2.1 Add `newFileExporter(path string) (sdktrace.SpanExporter, io.Closer, error)` to `internal/telemetry/telemetry.go` that opens `traces.jsonl` and wraps `stdouttrace.New(w)`
- [x] 2.2 Resolve trace file path: `$XDG_DATA_HOME/glitch/traces.jsonl` with fallback to `~/.local/share/glitch/traces.jsonl`; create parent dir if absent
- [x] 2.3 Replace the existing `stdouttrace.New(os.Stdout)` exporter with the file exporter in `Setup()`
- [x] 2.4 Ensure the file handle is closed in the returned `shutdown` func
- [x] 2.5 Add `traces.jsonl` and `*.jsonl` patterns to `.gitignore`
- [x] 2.6 Run a pipeline and confirm no OTel JSON appears in the TUI feed; confirm `traces.jsonl` is written

## 3. Wire span events into feed

- [x] 3.1 Define `TraceSpanMsg` in `internal/console/deck.go`: fields `RunID`, `StepID`, `SpanName`, `DurationMS int64`, `StatusOK bool`
- [x] 3.2 Add `DurationMS int64` field to `StepInfo` struct in `internal/console/deck.go`
- [x] 3.3 Implement `feedExporter` in `internal/telemetry/feed_exporter.go`: a `SpanExporter` that writes `TraceSpanMsg` to a `chan<- tea.Msg` on `ExportSpans()`; extract `run.id` and `step.id` from span attributes
- [x] 3.4 Register `feedExporter` alongside the file exporter using `sdktrace.NewMultiSpanExporter` in `Setup()`; expose the channel via a `FeedChan() <-chan tea.Msg` accessor
- [x] 3.5 In `deck.go`, receive `TraceSpanMsg` in `Update()`: find the matching `feedEntry` by run ID, find the matching `StepInfo` by step ID, set `DurationMS`
- [x] 3.6 Update feed step rendering (view layer) to display duration next to step badge when `DurationMS > 0`

## 4. /trace command

- [x] 4.1 Create `internal/console/trace_view.go` with `renderTraceTree(spans []traceSpan, width int) string` — indented lines showing `name · Xms · OK/ERR` using lipgloss styles from `internal/styles`
- [x] 4.2 Define `traceSpan` struct: `Name`, `DurationMS`, `OK bool`, `Depth int`
- [x] 4.3 Add `loadTraceForRun(tracesPath, runID string) ([]traceSpan, error)` — reads `traces.jsonl`, filters by `run.id` attribute, returns sorted spans
- [x] 4.4 Add `/trace` handler in the deck's slash-command switch: invoke `loadTraceForRun` for the selected feed entry's run ID; store result in model; re-render detail area
- [x] 4.5 Handle edge cases: no entry selected → inline "no run selected"; no spans found → inline "no trace data for this run"
- [x] 4.6 Run `go build ./...` and `go vet ./...` — zero errors gate
