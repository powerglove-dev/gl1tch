## Context

gl1tch's main TUI (`internal/console/switchboard.go`) is the operator's primary interface — it hosts the feed, chat panel, signal board, send panel, and all job lifecycle. It has accumulated the name "switchboard" which conflicts with the project's BBS/hacker identity. Separately, OTel tracing was added across orchestrator, pipeline, executor, and router but the `stdouttrace` exporter sends raw JSON to stdout, which flows into the TUI feed as noise. The feed already has `StepInfo` / `StepStatusMsg` / `FeedRunning/Done/Failed` states wired up — the missing piece is sourcing timing from spans and surfacing a trace view.

## Goals / Non-Goals

**Goals:**
- Rename `switchboard` → `deck` throughout `internal/console/` with zero behavior change
- Silence OTel stdout corruption by routing spans to `~/.glitch/traces.jsonl`
- Surface per-step `duration_ms` in feed entries, sourced from span end events
- Add `/trace` command that renders a span tree for the selected feed run

**Non-Goals:**
- Renaming the `console` package itself
- Shipping to an external OTLP collector (already handled by env var — no new work)
- Full distributed trace UI (Jaeger-style waterfall) — a text span tree is sufficient
- Instrumenting the BubbleTea TUI layer itself

## Decisions

### D1: Rename target is "deck", not the package

The `console` package name is fine — it's a generic container. The identifier that leaks everywhere is `switchboard` (file name, comments, variable names). Renaming just that term to `deck` isolates the blast radius to `internal/console/` and a handful of `cmd/` callers. The `Model` type stays as `Model`.

*Alternatives considered:* renaming the package to `deck` — would require updating every import across the codebase; not worth it for a cosmetic change.

### D2: File exporter writes to `~/.glitch/traces.jsonl`

OTel's `stdouttrace` writes to any `io.Writer`. We pass a file handle opened at `Setup()` time instead of `os.Stdout`. The file path follows XDG: `$XDG_DATA_HOME/glitch/traces.jsonl` (falls back to `~/.local/share/glitch/`). This matches where `glitch.db` already lives.

*Alternatives considered:* in-memory ring buffer exporter — more complex, no persistence for post-mortem; OTLP to a local collector — overkill for daily driver use.

### D3: Span → feed via `TraceSpanMsg` over the BUSD channel

OTel provides a `SpanExporter` interface. We implement a thin `feedExporter` that, on `ExportSpans()`, publishes a `TraceSpanMsg` (span ID, trace ID, name, duration, status, attributes) onto a buffered channel that the deck's `Update()` already polls (same pattern as `FeedLineMsg`). This keeps OTel completely decoupled from BubbleTea — no import of `tea` inside `internal/telemetry`.

*Alternatives considered:* polling `traces.jsonl` from the TUI — adds latency and file I/O on the hot path; using BUSD pub/sub — adds out-of-process hop for an in-process event.

### D4: `/trace` renders an inline span tree in the feed detail view

The feed's existing detail expansion (activated on selected entry) gains a second tab: "trace". It renders the span tree as indented lines with duration badges, reusing lipgloss styles already in `internal/styles`. No new panel type needed — the existing `inbox_detail.go` pattern applies.

*Alternatives considered:* a dedicated tmux window with `traces.jsonl` tailed — viable but breaks the TUI-native UX goal.

## Risks / Trade-offs

- **File handle leak on crash** → `telemetry.Setup()` returns a `shutdown` func already deferred in `main.go`; the file is closed there. Signal handling (`SIGINT`/`SIGTERM`) already triggers the deferred shutdown.
- **JSONL file grows unbounded** → out of scope for this change; rotation can be added later. File is human-readable and greppable in the interim.
- **`feedExporter` channel backpressure** → use a buffered channel (capacity 256); drop spans if full with a `log.Printf` warning. Spans are observability data, not control flow — dropping is acceptable.
- **Rename touches many files** → mechanical, low-risk. `go build ./...` is the acceptance gate. A single sed pass + manual review of exported symbols is sufficient.

## Migration Plan

1. Apply rename: `switchboard` → `deck` in all files under `internal/console/` and callers in `cmd/`
2. Swap telemetry exporter: replace `stdouttrace.New()` with `newFileExporter(tracesPath)` in `internal/telemetry/telemetry.go`
3. Add `feedExporter` in `internal/telemetry/feed_exporter.go`; register alongside file exporter as a `MultiSpanExporter`
4. Add `TraceSpanMsg` to `internal/console/deck.go`; wire channel into `Update()` and `feedEntry.steps`
5. Add `/trace` handler in deck's slash-command router; implement span tree renderer in `internal/console/trace_view.go`
6. `go build ./...` + `go vet ./...` gate

Rollback: revert commits — no schema migrations, no external state.

## Open Questions

- Should `/trace` show the full trace tree or just the steps for the selected feed entry? (Recommendation: scoped to the feed entry's run ID, not the global trace — keeps it focused.)
- Should `traces.jsonl` be gitignored by default? (Yes — add to `.gitignore`.)
