# Design — Elastic APM error surfacing

## Context

We already have:

- `internal/telemetry/telemetry.go` wiring a global OTel
  `TracerProvider` + `MeterProvider`
- `internal/telemetry/elasticsearch_exporter.go` writing spans directly
  to `glitch-traces` in our flat audit shape
- `pkg/glitchd/logbuffer.go` teeing every `slog` record into
  `glitch-logs` with workspace_id attribution
- `pkg/glitchd/telemetry.go` exposing `telemetry.Setup` to
  glitch-desktop through the internal-import boundary
- BatchSpanProcessor draining on `OnShutdown` so the desktop's
  `workspace.pod.start` and `collector.run` spans land in ES

We do NOT have:

- Kibana APM UI (no apm-server deployed)
- Error documents — panics and `slog.Error`s are plain log rows, not
  grouped error entities with stack traces and trace correlation
- Per-service grouping (`gl1tch-cli`, `gl1tch-desktop`, …) in a UI
  that understands services
- Per-tick child spans — today `collector.run` is one long parent span
  per goroutine lifetime. That's queryable as "did the goroutine
  launch" but not as "did the last tick finish". See § 6 and the
  follow-up change.

## Goals

1. Ship Go panics and `slog.Error`s to a queryable "Errors" view in
   Kibana without losing the flat-index debugging surface we already
   have.
2. Group errors by service so `gl1tch-desktop` crashes don't drown in
   `gl1tch-cli` pipeline-step failures.
3. Correlate every error to the span that was active when it fired, so
   "show me the logs for this crash" is a single click in Kibana.
4. Keep the whole thing opt-out-safe: if apm-server is down, the CLI
   and desktop keep running, `glitch-logs` keeps accumulating, and
   direct-to-ES spans still reach `glitch-traces`.

## Non-goals

- Replacing `glitch-logs` / `glitch-traces` with APM's own indices.
  Those stay. APM's schemas are optimized for APM's UI; our flat audit
  shape is optimized for the questions we ask in Kibana Discover.
- apm-rum (real-user monitoring of the frontend).
- Alerting / anomaly detection — that's a follow-up that needs a
  conversation about what we want to alert on.

## Decisions

### 1. apm-server deployment

Add a new service to `docker-compose.yml`:

```yaml
apm-server:
  image: docker.elastic.co/apm/apm-server:8.15.0   # match es/kibana version
  depends_on:
    - es
    - kibana
  ports:
    - "8200:8200"   # OTLP gRPC + HTTP + native APM intake
  environment:
    - output.elasticsearch.hosts=["http://es:9200"]
  command: >
    apm-server -e
      -E apm-server.host=0.0.0.0:8200
      -E apm-server.auth.anonymous.enabled=true
      -E apm-server.rum.enabled=false
      -E output.elasticsearch.hosts=["http://es:9200"]
```

**Anonymous auth is load-bearing for local dev.** We don't want to
bake an API key into local development; apm-server supports an
anonymous ingest mode that's fine for localhost. In production (if we
ever publish a hosted gl1tch) the apm-server config is a separate
problem and not covered here.

**Version lockstep with es/kibana.** Whatever Elastic stack version
`docker-compose.yml` pins for `es` and `kibana` is what apm-server
pins too. Mixed versions fail at startup with a clear error, which is
fine.

**Why not skip apm-server entirely and have gl1tch write directly to
`apm-*` indices?** Because the "APM processor" that normalizes spans
into error docs, transaction docs, and span docs lives in apm-server.
Bypassing it means re-implementing the doc shape plus the ILM policies
plus the grouping-key computation plus the service-map aggregation. We
are not doing that.

### 2. Dual-exporter trace path

Keep the existing custom `elasticsearchExporter` as one batcher, add a
standard OTLP gRPC exporter as a second batcher pointing at
`apm-server:8200`.

```go
// internal/telemetry/telemetry.go, inside Setup():

// Existing custom ES exporter — writes to glitch-traces flat index.
esExp := NewElasticsearchExporter(esClient, serviceName)
traceOpts = append(traceOpts, sdktrace.WithBatcher(esExp))

// New: OTLP exporter → apm-server → traces-apm-* + APM UI.
if apmEndpoint := os.Getenv("GL1TCH_APM_ENDPOINT"); apmEndpoint != "" {
    apmExp, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithInsecure(),
        otlptracegrpc.WithEndpoint(apmEndpoint),
    )
    if err != nil {
        slog.Warn("telemetry: apm exporter disabled", "err", err)
    } else {
        traceOpts = append(traceOpts, sdktrace.WithBatcher(apmExp))
        slog.Info("telemetry: apm exporter enabled", "endpoint", apmEndpoint)
    }
}
```

Default `GL1TCH_APM_ENDPOINT=localhost:8200` when the docker-compose
stack is up. Unset → the APM exporter is not wired and we fall back to
the legacy behavior. Two batchers adds a few hundred bytes of queue
memory and one extra goroutine per process — negligible.

**Why two exporters instead of "just use OTLP and read from
`traces-apm-*` in Kibana Discover"?** Because `traces-apm-*` docs are
deeply nested — `transaction.name`, `span.name`, `labels.*`,
`context.custom.*` — and every dashboard/query we already have for
the flat `glitch-traces` shape would need to be rewritten against the
nested schema. Keeping both gives us: Kibana APM UI for the human-
friendly error-and-service view, and the flat index for our existing
"what just happened in this process" queries.

### 3. Error capture: slog handler + explicit helper

Two entry points, one destination.

**Entry point A — slog.Error interceptor.** Extend `esTeeHandler` in
`pkg/glitchd/logbuffer.go` (or add a new sibling handler wired into
the same chain) so that when `r.Level >= slog.LevelError`, in
addition to the current enqueue-to-`glitch-logs` path, it also builds
an APM error doc and sends it to the apm-server intake endpoint.

Schema of the APM error doc we send:

```json
{
  "@timestamp": "2026-04-07T20:10:00.123456789Z",
  "service": {
    "name": "gl1tch-desktop",
    "version": "dev"
  },
  "error": {
    "id": "<ulid>",
    "culprit": "git collector: poll error",
    "grouping_key": "<sha1(service.name + culprit + error.exception.type)>",
    "log": {
      "level": "ERROR",
      "message": "git collector: poll error",
      "logger_name": "gl1tch"
    },
    "exception": {
      "type": "poll_error",
      "message": "git log: exit status 128",
      "stacktrace": [ /* optional; see below */ ]
    }
  },
  "labels": {
    "workspace_id": "82eeafcf-e474-4d95-8dc6-d68c886e623c",
    "collector": "git"
  },
  "trace": { "id": "<current trace id>" },
  "transaction": { "id": "<current transaction/span id>" }
}
```

Stacktrace is populated only for `telemetry.CaptureError` calls (entry
B). A plain `slog.Error("...", "err", err)` doesn't carry the Go stack
by default — we'd be fabricating one if we tried. That's a known
limitation and documented in the tasks.

**Entry point B — `telemetry.CaptureError(ctx, err, ...attrs)`.** An
explicit helper for the `recover()` blocks in:

- `internal/collector/pod.go:runCollectorGuarded`
- `internal/pipeline/runner.go` (pipeline step runner's panic guard)
- `internal/brain/service.go` (brain loop's panic guard)
- any future goroutine entry point with a `recover()`

This helper captures the current goroutine's stack via
`runtime.Callers`, extracts span + trace ID from ctx via the OTel API,
and sends the same APM error doc shape as entry A but with a populated
`error.exception.stacktrace` array.

Both entry points hit the same HTTP POST to
`http://apm-server:8200/intake/v2/events` with an NDJSON body. There
is NO separate go-client library — Elastic's Go APM agent
(`go.elastic.co/apm`) is a separate instrumentation stack that
duplicates a lot of what OTel is already doing for us. We send raw
NDJSON to the intake endpoint to keep the dependency footprint small.

### 4. Service naming

Three services, picked at `telemetry.Setup` time from the caller's
`serviceName` argument:

| Binary / context | `serviceName` |
|---|---|
| `glitch-desktop/main.go` → `SetupTelemetry(ctx, "gl1tch-desktop")` | `gl1tch-desktop` |
| `cmd/serve.go` headless → `telemetry.Setup(ctx, "gl1tch-cli")` | `gl1tch-cli` |
| pipeline runner subprocess | `gl1tch-pipeline` (NEW — today it inherits the parent process's service name) |

The pipeline runner change is the only code change needed here: when
a pipeline spawns a subprocess and that subprocess calls its own
`telemetry.Setup`, it passes `gl1tch-pipeline` as the service name so
pipeline step failures show up in the APM Errors view as their own
service and don't drown out desktop crashes.

### 5. Fallback + failure modes

- **apm-server down:** OTLP exporter returns errors on every batch
  tick, the SDK logs them once per batch, and the custom ES exporter
  keeps working. Error shipping via entry-point A/B fails silently
  (the NDJSON POST returns non-2xx and we log to stderr, NOT via slog,
  to avoid re-entering the error path).
- **ES down:** apm-server's own buffering kicks in for up to its
  configured queue size, then starts dropping. This is apm-server's
  problem, not ours — we accept the loss the same way we accept
  `glitch-logs` loss on ES outages.
- **Bad schema URL between SDK and semconv:** already fixed in this
  change's companion commit
  (`resource.NewSchemaless` instead of `Merge(Default, …)`).
- **Circular ERROR log on APM failure:** the slog handler must write
  apm-server POST failures directly to `os.Stderr`, not via
  `slog.Error`, or we'll loop the error back through ourselves. Same
  pattern as `logbuffer.go`'s bulk-index error reporting.

### 6. Relationship to per-tick child spans

Today `collector.run` is one parent span per goroutine lifetime and
only exports at pod stop / app shutdown. That's fine for "did the
goroutine launch" but useless for "did the last tick finish" and for
APM's transaction-duration histograms.

**This change does not fix that.** A separate follow-up
(`add-per-tick-collector-spans`) will introduce child spans
`collector.poll`, `collector.scan`, `collector.index` under the
existing `collector.run` parent. Each child span is short-lived
(one tick) and exports on the normal BatchSpanProcessor schedule, so
the APM Transactions view and the `traces-apm-*` index start showing
per-tick latency without requiring a pod restart.

The reason we sequence APM first: APM is where the per-tick spans
become valuable — in the flat `glitch-traces` index, we can already
query "last 10 ticks for workspace X" via a sort on `start_time`. In
APM, per-tick spans unlock histograms, latency distributions, and
error-rate-by-collector dashboards that are painful to reproduce by
hand. Doing the spans first without APM gives us data with nowhere
to render it.

The follow-up change is tracked as `add-per-tick-collector-spans` and
linked from `tasks.md`.

## Open questions

1. **Log shipping: OTLP logs vs NDSON intake?** OTel now has a logs
   data model and apm-server understands OTLP logs too. Sending the
   *same* `glitch-logs` stream to both ES (via our tee) and apm-server
   (via OTLP logs) would give us log↔trace correlation in the APM UI.
   Downside: double the log volume, double the failure modes. Deferred
   until we see what errors-only integration looks like in practice.
2. **ILM for `apm-*` indices.** apm-server ships a default ILM policy
   that deletes after 30 days. Fine for dev; production would want
   tuning. Not blocking.
3. **Sampling.** We currently `AlwaysSample()` every span. Once the
   per-tick spans land this might become expensive enough to warrant
   a parent-based sampler (e.g. sample errored spans always, keep 10%
   of successful ones). Separate conversation.
