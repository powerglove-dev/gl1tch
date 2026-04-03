---
title: "Telemetry"
description: "OpenTelemetry integration for distributed tracing and metrics in gl1tch pipelines."
order: 99
---

gl1tch uses OpenTelemetry (OTel) to instrument pipeline execution, capturing per-step timing, status, and game-state events for observability and debugging. Traces flow to file-based storage by default, or to an OTLP-compatible backend if configured; a custom feed exporter also streams span summaries to the TUI in real time.


## Architecture

Telemetry is initialized in `main.go` via `telemetry.Setup()`, which wires three exporters:

1. **Feed exporter** — routes span events to a buffered channel (capacity 256) consumed by the TUI's feed panel for real-time display of step completions and game events. Always active.
2. **File exporter** (default) — writes JSONL-formatted traces to `~/.local/share/glitch/traces.jsonl` (or `$XDG_DATA_HOME/glitch/traces.jsonl`). Used when `OTEL_EXPORTER_OTLP_ENDPOINT` is not set. Falls back silently if file I/O fails.
3. **OTLP gRPC exporter** (optional) — active only if `OTEL_EXPORTER_OTLP_ENDPOINT` is set; sends traces to a collector (e.g., Jaeger, Otel Collector, Datadog). Mutually exclusive with file exporter.

The feed exporter always runs; file and OTLP exporters are mutually exclusive (file is default, OTLP takes precedence if the endpoint is configured). The setup function returns a `shutdown` callback (already deferred in `main`) that flushes pending spans and closes file handles on exit.

Metrics are always exported to `~/.local/share/glitch/metrics.jsonl` in JSONL format. The metric reader polls the meter provider on a periodic interval.

Data flow:
- Pipeline steps and game logic create spans via `otel.Tracer().Start(ctx, spanName)`
- Spans are tagged with `run.id`, `step.id` (pipeline runs) or `game.*` attributes (game events)
- On span end:
  - Feed exporter receives the span in `ExportSpans()`, extracts key fields, and publishes to the buffered feed channel (256 capacity). If the channel is full, the span is silently dropped (observability data, not control flow).
  - File or OTLP exporter (whichever is active) batches the span and exports it in the background
- TUI feed panel drains the feed channel asynchronously, displaying updates in real time
- File and OTLP exporters use batch processors for efficiency (buffering before network/disk I/O)


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


## Traces and Metrics Emitted

### Pipeline Traces

Every pipeline run creates a root span with the pipeline name as its title. Each step in the pipeline becomes a child span under that root, tagged with:

- `run.id` — the unique pipeline run identifier
- `step.id` — the step's position or name within the run
- `span.name` — the step type and operation (e.g., `"shell.exec"`, `"llm.invoke"`, `"conditional.eval"`)

Attributes on pipeline spans vary by step type and are included in the full span data exported to files and external collectors.

### Game Spans

Game events emit spans with names prefixed by `game.` (e.g., `game.evaluate`, `game.unlock_achievement`). These are not tied to a specific run and are routed by name prefix. Common game span attributes:

- `game.ice_class` — set on `game.evaluate` spans when a cost or token threshold triggers; values include `"trace-ice"`, `"data-ice"`, etc.
- `game.achievements_count` — total achievements unlocked in the session
- `game.score_delta` — points gained or lost in that event

### Metrics

Metrics (counters, histograms, gauges) are exported to `metrics.jsonl`. The meter provider periodically exports aggregated metrics from application code. Common metric names:

- `pipeline.runs.total` — counter of completed pipeline runs
- `pipeline.steps.duration` — histogram of step execution times
- `llm.tokens.total` — counter of tokens consumed by model
- `game.achievements.total` — counter of achievements unlocked


## File Storage

Traces and metrics are stored as newline-delimited JSON (JSONL) in the local data directory:

```
~/.local/share/glitch/traces.jsonl    # OTel span records (or $XDG_DATA_HOME/glitch/traces.jsonl)
~/.local/share/glitch/metrics.jsonl   # Meter provider metrics
```

Both files are human-readable and greppable; file rotation is out of scope and can be added later. The directory is created automatically on first write.

Each line is a pretty-printed JSON object representing one span (traces) or metric point (metrics). You can parse these with `jq`:

```bash
jq '.name, .duration_ns, .status.code' ~/.local/share/glitch/traces.jsonl
```


## Configuration

### Environment Variables

**`OTEL_EXPORTER_OTLP_ENDPOINT`** — if set, enables OTLP gRPC export to this endpoint (e.g., `localhost:4317` for a local collector, or `api.datadoghq.com:443` for Datadog). If unset, traces go only to the local file. The collector must be reachable; connection errors are logged but do not block pipeline execution.

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

The feed exporter publishes to a buffered channel with capacity 256. If the channel fills (TUI consumer is slow or not running), new spans are dropped without blocking the pipeline or raising an error. This is acceptable because:
- Telemetry is observability data, not control flow
- Dropping some feed events does not affect pipeline correctness
- The default case in the `select` statement silently drops when the channel is full

If you notice feed events are missing in the TUI, check that the feed panel is actively draining the channel. You can also increase the channel capacity in `internal/telemetry/telemetry.go` if needed, though 256 is typically sufficient.


## TUI Integration

The TUI's feed panel drains the feed channel asynchronously, displaying step completions and game events as they arrive. Each `FeedSpanEvent` includes:

- `RunID`, `StepID` — for "pipeline" spans; empty for "game" spans
- `SpanName` — the operation name (e.g., `"shell.exec"`, `"game.evaluate"`)
- `DurationMS` — elapsed time in milliseconds
- `StatusOK` — true if the span ended without error
- `Kind` — `"pipeline"` or `"game"` (distinguishes routed event types)
- `GameICEClass`, `GameAchievementsCount` — set for "game" spans only

The `/trace` command (when implemented) will render the full span tree for a selected feed entry, scoped to that run's traces.

### What Gets Instrumented Automatically

Built-in pipeline steps emit spans automatically:
- **Shell steps** (`shell.exec`) — start, end, exit code, command, output
- **LLM steps** (`llm.invoke`) — model, provider, prompt tokens, completion tokens, temperature
- **Conditional steps** (`conditional.eval`) — result, latency
- **Game events** (`game.evaluate`, `game.unlock_achievement`) — ICE class, achievements count

Custom pipeline steps (Go or shell) may need manual instrumentation. See [Pipeline steps](/docs/pipelines.md) for how to emit spans from custom code.


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

Traces will appear in the Datadog APM UI under your service name (`"gl1tch"`). Ensure your Datadog API key is configured in your environment (Datadog agent or OTLP endpoint configuration).

### Tempo

For self-hosted distributed tracing with Grafana Tempo, deploy Tempo with its OTLP gRPC receiver, then:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=tempo.local:4317 glitch
```

Traces will be queryable in Grafana and integrated with Loki logs if configured.

## Debugging Telemetry

### Inspect What Traces Are Being Created

When a pipeline runs, spans are emitted for each operation. Check what traces are being recorded:

```bash
# Watch traces as they're written (tail -f + jq)
tail -f ~/.local/share/glitch/traces.jsonl | jq '.name, .attributes'

# Count traces by span name
jq '.name' ~/.local/share/glitch/traces.jsonl | sort | uniq -c

# List all unique run IDs (one pipeline run per ID)
jq '.attributes | select(.["run.id"]) | .["run.id"]' ~/.local/share/glitch/traces.jsonl | sort | uniq

# Show all spans for a specific run
RUN_ID="abc123" && jq "select(.attributes[\"run.id\"] == \"$RUN_ID\")" ~/.local/share/glitch/traces.jsonl
```

### Analyze Performance

Find performance bottlenecks by examining duration:

```bash
# Find slow spans (> 1 second = 1,000,000,000 ns)
jq 'select((.endTime - .startTime) > 1000000000) | {name, duration_ns: (.endTime - .startTime)}' ~/.local/share/glitch/traces.jsonl

# Find slowest 10 spans
jq '{name, duration_ns: (.endTime - .startTime)}' ~/.local/share/glitch/traces.jsonl | jq -s 'sort_by(.duration_ns) | reverse | .[0:10]'

# Calculate median duration by span name
jq '{name, duration_ms: ((.endTime - .startTime) / 1000000)}' ~/.local/share/glitch/traces.jsonl | jq -s 'group_by(.name) | map({name: .[0].name, median_ms: ([.[].duration_ms] | sort | .[length/2])})'
```

### Troubleshoot Missing Traces

If traces are not appearing in the feed or file:

1. **Check if Setup() was called:** Verify `telemetry.Setup()` is called early in `main.go` and the shutdown function is deferred. Without it, no exporters are wired.

2. **Check if OTLP endpoint is reachable:** If `OTEL_EXPORTER_OTLP_ENDPOINT` is set but the endpoint is unreachable, the OTLP exporter logs an error to stderr but does not block pipeline execution. When OTLP is enabled, traces are not written to the file exporter (they are mutually exclusive).

3. **Check file permissions:** If `~/.local/share/glitch/` is not writable, the file exporter fails silently. Verify the directory exists and is writable:
   ```bash
   mkdir -p ~/.local/share/glitch && touch ~/.local/share/glitch/traces.jsonl
   ```

4. **Check sampling:** If using `TraceIDRatioBased(0.1)`, only 10% of traces are exported. Use `AlwaysSample()` during debugging to see every span.

5. **Check channel capacity:** If the TUI is not running or is slow to drain the feed channel, spans may be dropped. Monitor with:
   ```bash
   # See if spans are arriving faster than they're being consumed
   watch -n 1 'wc -l ~/.local/share/glitch/traces.jsonl'
   ```

### Inspect Metrics

Metrics are recorded separately from traces:

```bash
# Pretty-print all metrics
cat ~/.local/share/glitch/metrics.jsonl | jq '.'

# Find counters by name
jq 'select(.type == "Counter")' ~/.local/share/glitch/metrics.jsonl

# Export metrics to CSV
jq -r '[.name, .timestamp, .value] | @csv' ~/.local/share/glitch/metrics.jsonl > metrics.csv
```

### Local File Analysis

For offline analysis without an external observability platform:

```bash
# Pretty-print all traces
cat ~/.local/share/glitch/traces.jsonl | jq '.'

# Find all error spans
jq 'select(.status.code == "Error")' ~/.local/share/glitch/traces.jsonl

# Export traces to CSV for analysis
jq -r '[.name, .startTime, (.endTime - .startTime), .status.code] | @csv' ~/.local/share/glitch/traces.jsonl > traces.csv

# Extract attributes from a specific span
jq '.attributes' ~/.local/share/glitch/traces.jsonl | head -1
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

Edit `internal/telemetry/telemetry.go` and change the sampling configuration from:

```go
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))
```

to:

```go
traceOpts = append(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)))
```

Then rebuild gl1tch. Only 1 in 10 pipeline runs will now export full traces (trace IDs are sampled before the decision, so sampling is consistent across all exporters). During development, keep `AlwaysSample()` to see every span.

### Debug with verbose exporter output

To see exporter errors (e.g., OTLP connection failures):

```bash
# Look for stderr output from otel exporters when pipeline runs:
glitch 2>&1 | grep -i "otel\|telemetry"
```

If the OTLP exporter fails to connect, you'll see a message on stderr, but the pipeline continues.

### Query local traces

```bash
# Pretty-print all traces
cat ~/.local/share/glitch/traces.jsonl | jq '.'

# Find all errors
jq 'select(.status.code == "Error")' ~/.local/share/glitch/traces.jsonl

# Export to CSV for analysis
jq -r '[.name, .startTime, (.endTime - .startTime), .status.code] | @csv' ~/.local/share/glitch/traces.jsonl > traces.csv
```


## Edge Cases and Limitations

**File I/O failures:** If the traces file cannot be written (permissions, disk full, etc.), the file exporter fails silently and traces are discarded. Ensure `~/.local/share/glitch/` is writable before running gl1tch in production.

**OTLP connection failures:** If `OTEL_EXPORTER_OTLP_ENDPOINT` is set but unreachable, the OTLP exporter logs an error to stderr but does not block pipeline execution. Traces are lost for that run since file export is disabled when OTLP is configured.

**Feed channel overflow:** If the buffered channel (capacity 256) fills, spans are silently dropped. This is acceptable for observability but means you may miss events in the TUI feed if the consumer is slow. Monitor with `tail -f traces.jsonl | wc -l`.

**No trace continuity across process restarts:** Traces are exported on shutdown, but in-flight spans at crash time are lost. Always call the shutdown function returned by `Setup()` in a deferred statement.

**Metrics export latency:** Metrics use a periodic reader, so recent metric data may not appear in `metrics.jsonl` until the next export tick.


## See Also

- [Pipeline steps](/docs/pipelines.md) — how to emit spans from shell and Go steps
- [Game system](/docs/game.md) — game events and achievement tracking (uses telemetry spans)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/) — semantic conventions, OTLP spec, exporter guides

