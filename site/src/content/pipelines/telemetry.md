---
title: "Telemetry"
description: "OpenTelemetry integration for distributed tracing and metrics in gl1tch pipelines."
order: 99
---

gl1tch uses OpenTelemetry (OTel) to instrument pipeline execution, capturing per-step timing, status, and game-state events for observability and debugging. Traces flow to file-based storage by default, or to an OTLP-compatible backend if configured; a custom feed exporter also streams span summaries to the TUI in real time.


## Architecture

Telemetry is initialized in `main.go` via `telemetry.Setup()`, which wires three exporters:

1. **File exporter** — writes JSONL-formatted traces to `~/.local/share/glitch/traces.jsonl` (or `$XDG_DATA_HOME/glitch/traces.jsonl`). Falls back silently if file I/O fails.
2. **OTLP gRPC exporter** — active only if `OTEL_EXPORTER_OTLP_ENDPOINT` is set; sends traces to a collector (e.g., Jaeger, Otel Collector, Datadog).
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

**Run ID** (`run.id`) — unique identifier for a single pipeline execution; links all steps in that run.

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

Each line is a pretty-printed JSON object representing one span (traces) or metric point (metrics). You can parse these with `jq`:

```bash
jq '.name, .duration, .status.code' ~/.local/share/glitch/traces.jsonl
```


## Configuration

### Environment Variables

**`OTEL_EXPORTER_OTLP_ENDPOINT`** — if set, enables OTLP gRPC export to this endpoint (e.g., `localhost:4317` for a local collector). If unset, traces go only to the local file. The collector must be reachable; connection errors are logged but do not block pipeline execution.

**`XDG_DATA_HOME`** — base directory for trace and metric files. Defaults to `$HOME/.local/share` if not set.


### Sampling and Resource

All spans are sampled (`AlwaysSample()`), meaning every span is exported. To change the sampling strategy, modify the `sdktrace.WithSampler()` call in `internal/telemetry/telemetry.go`:

```go
// Always sample (current):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))

// Sample only 10% of traces:
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)))

// Never sample (only feed events):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.NeverSample()))
```

The tracer provider is configured with a service resource containing:

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


## Integration with External Observability Platforms

### Jaeger

To send traces to Jaeger, start a local collector and point gl1tch at it:

```bash
# Start Jaeger all-in-one (includes collector on port 4317)
docker run -p 4317:4317 jaegertracing/all-in-one:latest

# In another terminal, run gl1tch with OTLP export enabled
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 glitch
```

Navigate to `http://localhost:16686` to view traces in the Jaeger UI.

### OpenTelemetry Collector

Deploy an OTel Collector as a sidecar or standalone service, then configure gl1tch to export to it:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=collector.local:4317 glitch
```

The collector can then export to Jaeger, Tempo, Datadog, or any other OTLP-compatible backend.

### Local File Analysis

If you do not have an external observability platform, traces are always written locally. Analyze them with command-line tools:

```bash
# Count traces by span name
jq '.name' ~/.local/share/glitch/traces.jsonl | sort | uniq -c

# Find slow spans (> 1 second)
jq 'select((.endTime - .startTime) > 1000000000) | {name, duration_ns: (.endTime - .startTime)}' ~/.local/share/glitch/traces.jsonl

# Extract run IDs and their total duration
jq '.attributes | select(.["run.id"]) | .["run.id"]' ~/.local/share/glitch/traces.jsonl | sort | uniq
```


## Examples

### Enable OTLP export to local collector

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
glitch
```

After running a pipeline, verify traces arrived at the collector:

```bash
# Check Jaeger (if running)
curl http://localhost:16686/api/services | jq '.data[] | select(.name == "gl1tch")'
```

### Change sampling to 10%

Edit `internal/telemetry/telemetry.go` and change the sampler from:

```go
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))
```

to:

```go
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)))
```

Then rebuild gl1tch. Only 1 in 10 pipeline runs will now export full traces (trace IDs are sampled before the decision, so sampling is consistent across all exporters).

### Query local traces

```bash
# Pretty-print all traces
cat ~/.local/share/glitch/traces.jsonl | jq '.'

# Find all errors
jq 'select(.status.code == "Error")' ~/.local/share/glitch/traces.jsonl

# Export to CSV for analysis
jq -r '[.name, .startTime, (.endTime - .startTime), .status.code] | @csv' ~/.local/share/glitch/traces.jsonl > traces.csv
```


## See Also

- [Pipeline steps](/docs/pipelines.md) — how to emit spans from shell and Go steps
- [Game system](/docs/game.md) — game events and achievement tracking (uses telemetry spans)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/) — semantic conventions, OTLP spec, exporter guides

