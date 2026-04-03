---
title: "Telemetry"
description: "OpenTelemetry integration for distributed tracing and metrics in gl1tch pipelines."
order: 99
---

gl1tch uses OpenTelemetry (OTel) to instrument pipeline execution, capturing per-step timing, status, and game-state events for observability and debugging. Traces flow to file-based storage by default, or to an OTLP-compatible backend if configured; a custom feed exporter also streams span summaries to the TUI in real time for live feedback.


## Architecture

Telemetry is initialized in `main.go` via `telemetry.Setup()`, which wires three parallel exporters:

1. **Feed exporter** — routes span summaries to a buffered channel (capacity 256) consumed by the TUI's feed panel for real-time display of step completions and game events. Always active. Implemented in `internal/telemetry/feed_exporter.go`.

2. **File exporter** (default) — writes JSONL-formatted traces to `~/.local/share/glitch/traces.jsonl` (or `$XDG_DATA_HOME/glitch/traces.jsonl`). Used when `OTEL_EXPORTER_OTLP_ENDPOINT` is not set. Falls back silently if file I/O fails.

3. **OTLP gRPC exporter** (optional) — active only if `OTEL_EXPORTER_OTLP_ENDPOINT` is set; sends traces to a collector (e.g., Jaeger, OTel Collector, Datadog). Connection failures are logged but do not block pipeline execution.

The feed exporter always runs; file and OTLP exporters are mutually exclusive (file is default, OTLP takes precedence if the endpoint is configured). The setup function returns a `shutdown` callback (deferred in `main.go`) that flushes pending spans and closes file handles on exit or signal.

Metrics are always exported to `~/.local/share/glitch/metrics.jsonl` in JSONL format via a periodic meter provider reader.

Data flow:
- Pipeline steps and game logic create spans via `otel.Tracer().Start(ctx, spanName)`
- Spans are tagged with `run.id`, `step.id` (pipeline runs) or `game.*` attributes (game events)
- On span end:
  - Feed exporter (`FeedExporter.ExportSpans()`) extracts key fields and publishes `FeedSpanEvent` to the buffered channel. If the channel is full (TUI consumer slow), the span is silently dropped (observability data, not control flow).
  - File or OTLP exporter (whichever is active) batches the span via `BatchSpanProcessor` and exports it in the background
- TUI feed panel drains the feed channel asynchronously, displaying updates in real time
- Both file and OTLP exporters use batch processors for efficiency (buffering before disk/network I/O)


## Technologies

**OpenTelemetry SDK for Go** — Provides the standard tracing and metrics API (`go.opentelemetry.io/otel`). gl1tch uses:
- `otel.Tracer()` to create spans from pipeline steps and game logic
- `sdktrace.BatchSpanProcessor` to batch span exports for efficiency (reduces I/O)
- `stdouttrace` exporter for JSONL file output (no external binaries required)
- `otlptrace/otlptracegrpc` for remote OTLP endpoints (Jaeger, Tempo, Datadog, etc.)
- `sdktrace.AlwaysSample()` sampler to export every span (fully configurable)

**Semantic Conventions** — OTel standard attribute names (e.g., `service.name`, `service.version`) enable observability tools to filter and group spans across systems. gl1tch uses OpenTelemetry v1.26.0 semantic conventions.

**Structured Data on the Wire** — Traces are newline-delimited JSON, human-readable and queryable with `jq`. This choice avoids binary dependencies and enables offline analysis, tooling, auditing, and git-friendly diffs.


## Concepts

**Span** — A unit of work with a start time, end time, status code, and attributes. Each pipeline step creates a span; nested operations (LLM calls, shell execution) create child spans.

**Trace** — A tree of related spans rooted at a pipeline run. The trace ID is the run ID; parent-child relationships are tracked via span IDs.

**Run ID** (`run.id`) — Unique identifier for a single pipeline execution; links all steps in that run.

**Step ID** (`step.id`) — Unique identifier for a single step within a run; used by TUI to map spans to feed entries.

**Attributes** — Key-value metadata attached to a span (e.g., `llm.model`, `shell.exit_code`, `game.ice_class`). Attributes are searchable and filterable in observability backends.

**Kind** — `"pipeline"` spans are identified by run ID and step ID (typical for pipeline runs). `"game"` spans are identified by span name prefix (e.g., `"game.evaluate"`) and used for game-state events.

**FeedSpanEvent** — A minimal span summary struct sent to the TUI feed channel, containing:
- `RunID`, `StepID` — for "pipeline" spans
- `SpanName` — the operation name (e.g., `"shell.exec"`, `"llm.generate"`, `"game.evaluate"`)
- `DurationMS` — elapsed time in milliseconds
- `StatusOK` — true if span ended without error
- `Kind` — `"pipeline"` or `"game"`
- `GameICEClass`, `GameAchievementsCount` — set for "game" spans only

**Game span** — A span with name starting with `game.` (e.g., `"game.evaluate"`, `"game.ice"`). Not tied to a run or step; routed by name prefix.

**Sampler** — Decides which spans are exported. `AlwaysSample()` exports everything; `TraceIDRatioBased(0.1)` exports 10%. Sampling trades observability for performance; only affects file and OTLP exporters (feed always publishes).


## What Gets Instrumented

**Pipeline Execution** — Every pipeline run creates a root span with the run ID as the trace ID. Each step (shell, LLM, conditional, fork) generates a child span with:
- Step ID and provider name
- Step type (shell, llm, conditional, fork, etc.)
- Latency (start and end time, computed as `endTime - startTime`)
- Status code (OK or error)
- Provider-specific attributes (e.g., model name, token counts, exit code, command)

**LLM Calls** — When a pipeline invokes an LLM provider, detailed spans capture:
- Model name and temperature
- Prompt length (in tokens)
- Input and output token counts
- Latency (time-to-first-token if available, total latency)
- Error details and retry count if the call fails

**Game Events** — Game logic emits spans prefixed with `"game."` (e.g., `"game.evaluate"`, `"game.ice"`). These carry game-specific attributes:
- `game.ice_class` — the faction or class triggered (e.g., `"trace-ice"`, `"data-ice"`)
- `game.achievements_count` — number of achievements unlocked in this run
- `game.score_delta` — points gained or lost (if applicable)

**Shell Execution** — Shell step spans include:
- Command executed
- Exit code (0 for success, non-zero for failure)
- Stdout/stderr (metadata only; full content not captured in spans for privacy)
- Wall-clock duration

Custom pipeline steps can emit spans via `otel.Tracer().Start(ctx, spanName)` and attach attributes with `span.SetAttributes()`.


## Configuration

### Environment Variables

**`OTEL_EXPORTER_OTLP_ENDPOINT`** — If set, enables OTLP gRPC export to this endpoint. Examples:
- `localhost:4317` — local OTel Collector
- `api.datadoghq.com:443` — Datadog APM backend (requires `OTEL_EXPORTER_OTLP_HEADERS=dd-api-key=...`)
- `tempo.mycompany.com:4317` — self-hosted Grafana Tempo
- `otlp.example.com:4317` — any OTLP-compatible backend

If unset, traces go only to the local file. Connection errors are logged but do not block pipeline execution.

**`XDG_DATA_HOME`** — Base directory for trace and metric files. Defaults to `$HOME/.local/share` if not set. gl1tch creates `$XDG_DATA_HOME/glitch/` automatically.

**`OTEL_SERVICE_NAME`** — Reserved for future use. gl1tch currently hardcodes `service.name` to `"gl1tch"`.

### Sampling Strategy

By default, all spans are sampled (`AlwaysSample()`). To change sampling, edit `internal/telemetry/telemetry.go` and modify the sampler in `Setup()`:

```go
// Always sample (current):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))

// Sample 10% of traces (reduce file I/O and storage):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)))

// Sample 1% of traces (only heavy pipelines):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.01)))

// Never sample (only feed events, no file output):
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.NeverSample()))
```

Then rebuild with `go build ./...` and restart gl1tch. Sampling only affects file and OTLP exporters; the feed exporter always publishes to its channel.

### Resource Attributes

Every span carries these resource attributes:
- `service.name` — `"gl1tch"` (hardcoded)
- `service.version` — `"dev"` (hardcoded)

This enables filtering and grouping by service in observability backends.


## Feed Channel and Backpressure

The feed exporter publishes `FeedSpanEvent` structs to a buffered channel with capacity 256. If the channel fills (TUI feed consumer is slow or blocked), new events are dropped without error. This is acceptable because telemetry is observability data, not control flow; dropping some feed events does not affect pipeline correctness or user experience.

**Why not block?** — Blocking would stall pipeline execution while waiting for TUI to drain the channel. Dropping is safer; an occasional missed feed event is better than a hung pipeline.


## TUI Integration

### Feed Panel Display

The TUI's feed panel drains the feed channel asynchronously, displaying step completions and game events in real time. Each feed entry shows:
- Step status badge (`✓` OK, `✗` error, `·` pending)
- Duration in milliseconds (from span duration)
- Step name or game event name
- Provider name (for pipeline spans)

### OTel Trace View

Press `o` in the Inbox Detail modal to view the full trace tree for a selected run. The trace view:
- Parses `~/.local/share/glitch/traces.jsonl`
- Filters spans to the current run ID
- Renders the span tree indented by depth
- Shows status (OK or ERR) and duration (ms) for each span

Example trace tree:
```
fetch-schema                                    OK · 850ms
  ├ http.request                                OK · 145ms
  ├ http.response                               OK · 12ms
  └ json.parse                                  OK · 3ms
transform                                       OK · 680ms
  ├ schema.validate                             OK · 450ms
  └ schema.transform                            OK · 230ms
publish                                         OK · 120ms
  └ http.request                                OK · 115ms
```

This view is useful for diagnosing performance bottlenecks in slow pipelines and understanding the call hierarchy of complex runs.


## Reading Traces

Trace files are JSONL (one JSON object per line). Query them with `jq`:

```bash
# Pretty-print all traces
cat ~/.local/share/glitch/traces.jsonl | jq '.'

# Show span name, duration, and status
jq '{name: .name, durationMs: (.endTime - .startTime) / 1000000, status: .status.code}' \
  ~/.local/share/glitch/traces.jsonl

# Find all error spans
jq 'select(.status.code == "Error")' ~/.local/share/glitch/traces.jsonl

# Count spans by name (find most common operations)
jq '.name' ~/.local/share/glitch/traces.jsonl | sort | uniq -c | sort -rn

# Find spans by run ID
jq 'select(.attributes."run.id" == "abc123")' ~/.local/share/glitch/traces.jsonl

# Find slow spans (> 1 second)
jq 'select((.endTime - .startTime) > 1000000000) | {name, durationMs: (.endTime - .startTime) / 1000000}' \
  ~/.local/share/glitch/traces.jsonl

# Count tokens by model
jq -r '[.attributes."llm.model", .attributes."llm.tokens.output"] | select(.[0]) | @csv' \
  ~/.local/share/glitch/traces.jsonl | sort | uniq -c

# Export to CSV for analysis
jq -r '[.name, .startTime, (.endTime - .startTime), .status.code] | @csv' \
  ~/.local/share/glitch/traces.jsonl > traces.csv
```

### Metrics

Metrics are exported to `~/.local/share/glitch/metrics.jsonl` in the same JSONL format. The metric reader polls on a periodic interval (configurable in `telemetry.Setup()`). Query metrics similarly with `jq`.


## Instrumentation Guide

### Emitting Spans from Custom Steps

Custom pipeline steps can emit spans via the `otel.Tracer()` API:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

func MyCustomStep(ctx context.Context, input interface{}) (interface{}, error) {
    tracer := otel.Tracer("my.provider")
    ctx, span := tracer.Start(ctx, "my.operation")
    defer span.End()

    // Attach attributes
    span.SetAttributes(
        attribute.String("step.id", stepID),
        attribute.String("my.param", "value"),
        attribute.Int64("my.metric", 42),
    )

    // Do work...
    result, err := doWork(ctx)

    // Errors are automatically captured
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    }

    return result, err
}
```

Attributes are searchable and filterable in observability backends. Use lowercase keys with dots (semantic conventions style): `service.name`, `http.method`, `db.statement`, etc.

### Emitting Game Events

Game logic can emit spans with the `"game."` prefix:

```go
tracer := otel.Tracer("game")
ctx, span := tracer.Start(ctx, "game.ice")
defer span.End()

span.SetAttributes(
    attribute.String("game.ice_class", "faction_rebel"),
    attribute.Int64("game.achievements_count", 3),
)
```

The feed exporter automatically routes spans with `"game."` prefix to the TUI feed display.


## File Locations and Cleanup

**Trace File** — `~/.local/share/glitch/traces.jsonl`
- Stores all exported spans (if file exporter is active)
- Appends on each run
- Grows unbounded; manual rotation required for long sessions

**Metrics File** — `~/.local/share/glitch/metrics.jsonl`
- Stores aggregated metrics
- Also appends; also requires rotation

**Storage Location**
- Respects `$XDG_DATA_HOME/glitch/` if set
- Falls back to `~/.local/share/glitch/` otherwise
- Directory is created automatically on first write

### Rotation and Archival

For long-running sessions, rotate or archive old traces:

```bash
# Rotate traces (keep last 10MB)
tail -c 10M ~/.local/share/glitch/traces.jsonl > ~/.local/share/glitch/traces.jsonl.tmp
mv ~/.local/share/glitch/traces.jsonl.tmp ~/.local/share/glitch/traces.jsonl

# Archive old traces (gzip)
gzip -c ~/.local/share/glitch/traces.jsonl > ~/.local/share/glitch/traces-$(date +%Y%m%d).jsonl.gz

# Check disk usage
du -h ~/.local/share/glitch/
```

Removing the files does not affect running gl1tch instances; new traces will be written on the next pipeline execution.


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

### Datadog

Datadog accepts OTLP gRPC traces at their intake endpoint. Set:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http-intake.logs.datadoghq.com:443 glitch
```

Traces will appear in the Datadog APM UI under your service name (`"gl1tch"`).

### Tempo

For self-hosted distributed tracing with Grafana Tempo, deploy Tempo with its OTLP gRPC receiver, then:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=tempo.local:4317 glitch
```

Traces will be queryable in Grafana and integrated with Loki logs if configured.

### Local File Analysis

If you do not have an external observability platform, traces are always written locally. Analyze them with command-line tools (see [Reading Traces](#reading-traces) section above).


## Troubleshooting

**Traces not appearing in file:**
- Check `$XDG_DATA_HOME` or `~/.local/share/glitch/` exists and is writable: `ls -ld ~/.local/share/glitch/`
- Run a pipeline and check the file exists: `ls -lh ~/.local/share/glitch/traces.jsonl`
- Look for errors in stderr during setup (rare, but possible)

**OTLP export failing:**
- Verify the endpoint is reachable: `nc -zv localhost 4317` (or use appropriate hostname/port)
- Check logs for connection errors; they will not stop pipeline execution
- Try a simple test: start `jaeger all-in-one` locally (see Jaeger section above)

**Feed not updating in TUI:**
- Ensure a pipeline is running and spans are completing
- Check the feed panel is not scrolled past the new entries
- Verify the feed exporter channel is not dropping events (would require TUI paused for >256 spans)

**File too large:**
- Rotate or gzip as shown in [File Locations and Cleanup](#file-locations-and-cleanup) section above
- Consider enabling sampling to reduce trace volume
- Archive old traces after analysis

**jq queries not working:**
- Verify jq is installed: `which jq`
- Check the file is valid JSON: `jq . ~/.local/share/glitch/traces.jsonl | head`
- Use `jq '.name'` to see span names available in your traces


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

Edit `internal/telemetry/telemetry.go` and modify the sampler in `Setup()`:

```go
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)))
```

Then rebuild gl1tch. Only 1 in 10 pipeline runs will now export full traces (trace IDs are sampled consistently across all exporters).

### Query local traces

```bash
# Pretty-print all traces
cat ~/.local/share/glitch/traces.jsonl | jq '.'

# Find all errors
jq 'select(.status.code == "Error")' ~/.local/share/glitch/traces.jsonl

# Export to CSV for analysis
jq -r '[.name, .startTime, (.endTime - .startTime), .status.code] | @csv' \
  ~/.local/share/glitch/traces.jsonl > traces.csv
```


## See Also

- [Pipeline steps](/docs/pipelines.md) — how to emit spans from shell and Go steps
- [Game system](/docs/game.md) — game events and achievement tracking (uses telemetry spans)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/) — semantic conventions, OTLP spec, exporter guides
- [Jaeger Getting Started](https://www.jaegertracing.io/docs/getting-started/) — local trace backend for development
