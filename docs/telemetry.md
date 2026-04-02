---
title: "Telemetry"
description: "OpenTelemetry integration for distributed tracing and metrics in gl1tch pipelines."
order: 99
---

gl1tch uses OpenTelemetry (OTel) to instrument pipeline execution, capturing per-step timing, status, and game-state events for observability and debugging. Traces flow to file-based storage by default, or to an OTLP-compatible backend if configured; a custom feed exporter also streams span summaries to the TUI in real time.


## Architecture

Telemetry is initialized in `main.go` via `telemetry.Setup()`, which wires three exporters:

1. **File exporter** — writes JSONL-formatted traces to `~/.local/share/glitch/traces.jsonl` (or `$XDG_DATA_HOME/glitch/traces.jsonl`). Falls back silently if file I/O fails.
2. **OTLP gRPC exporter** — active only if `OTEL_EXPORTER_OTLP_ENDPOINT` is set; sends traces to a collector (e.g., Jaeger, Otel Collector).
3. **Feed exporter** — routes span events to a buffered channel (capacity 256) consumed by the TUI's feed panel for real-time display of step completions and game events.

All three operate in parallel. The setup function returns a `shutdown` callback (already deferred in `main`) that flushes pending spans and closes file handles on exit.

Metrics are always exported to `~/.local/share/glitch/metrics.jsonl` in JSONL format. The metric reader polls the meter provider on a periodic tick.

Data flow:
- Pipeline steps and game logic create spans via `otel.Tracer().Start(ctx, spanName)`
- Spans are tagged with `run.id`, `step.id` (pipeline runs) or `game.*` attributes (game events)
- On span end, all exporters receive the span in `ExportSpans()`
- Feed exporter extracts key fields and publishes to the feed channel; TUI drains this channel asynchronously
- File and OTLP exporters use batch processors for efficiency


## Technologies

- **OpenTelemetry SDK** — distributed tracing and metrics APIs; semantic conventions for resource attributes.
- **OTLP gRPC** — wire protocol for exporting traces to external collectors (opt-in).
- **Standard output exporters** — JSONL writers for local file storage (no external dependencies).


## Concepts

**Span** — a unit of work with a start time, end time, and status. A span represents one step in a pipeline run or one game event (achievement unlock, ICE trigger, etc).

**Run ID** (`run.id`)** — unique identifier for a single pipeline execution; links all steps in that run.

**Step ID** (`step.id`) — unique identifier for a single step within a run; used by TUI to map spans to feed entries.

**Game span** — a span with name starting with `game.` (e.g., `game.evaluate`, `game.tune`). Not tied to a run or step; routed by name prefix.

**Feed event** — a lightweight summary of a span (name, duration, status, run/step ID, kind) sent to the TUI feed channel. The feed exporter produces these on every span end, allowing the TUI to display step completions in near real time.

**ICE class** (`game.ice_class`) — attribute on a `game.evaluate` span indicating which cost/data threshold was triggered. Values: `"trace-ice"` (cost exceeded), `"data-ice"` (token count exceeded), etc.

**Achievements count** (`game.achievements_count`) — attribute on game spans showing how many achievements were unlocked in that run.


## File Storage

Traces and metrics are stored as newline-delimited JSON (JSONL) in the local data directory:

```
~/.local/share/glitch/traces.jsonl    # OTel span records (or $XDG_DATA_HOME/glitch/traces.jsonl)
~/.local/share/glitch/metrics.jsonl   # Meter provider metrics
```

Both files are human-readable and greppable; file rotation is out of scope and can be added later. The directory is created automatically on first write.


## Environment Variables

**`OTEL_EXPORTER_OTLP_ENDPOINT`** — if set, enables OTLP gRPC export to this endpoint (e.g., `localhost:4317`). If unset, traces go only to the local file.

**`XDG_DATA_HOME`** — base directory for trace and metric files. Defaults to `$HOME/.local/share` if not set.


## Sampling and Resource

All spans are sampled (`AlwaysSample()`), meaning every span is exported. The tracer provider is configured with a service resource containing:

- `service.name` — set from the `serviceName` arg to `Setup()` (typically `"gl1tch"`)
- `service.version` — hardcoded to `"dev"`

These attributes appear in every exported span, enabling filtering and grouping by service in observability backends.


## Feed Channel Backpressure

The feed exporter publishes to a buffered channel with capacity 256. If the channel fills (TUI consumer is slow), new spans are dropped without error. This is acceptable because telemetry is observability data, not control flow; dropping some feed events does not affect pipeline correctness. A `log.Printf` warning is issued when dropping occurs (currently commented out in the code but may be added for diagnostics).


## TUI Integration

The TUI's feed panel drains the feed channel asynchronously, displaying step completions and game events as they arrive. Each `FeedSpanEvent` includes:

- `RunID`, `StepID` — for "pipeline" spans; empty for "game" spans
- `SpanName` — the operation name (e.g., `"shell.exec"`, `"game.evaluate"`)
- `DurationMS` — elapsed time in milliseconds
- `StatusOK` — true if the span ended without error
- `Kind` — `"pipeline"` or `"game"` (distinguishes routed event types)
- `GameICEClass`, `GameAchievementsCount` — set for "game" spans only

The `/trace` command (when implemented) will render the full span tree for a selected feed entry, scoped to that run's traces.


## See Also

- [Pipeline steps](/docs/pipelines.md) — how to emit spans from shell and Go steps
- [Game system](/docs/game.md) — game events and achievement tracking (uses telemetry spans)
