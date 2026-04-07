# Elastic APM error surfacing

## Why

`glitch-logs` and `glitch-traces` now ship from both the CLI and the desktop
via the OTel stack. That gives us the raw substrate — every `slog.Error`
lands as a log doc, every collector tick is wrapped in a span — but it
doesn't give us the two things we actually want when something breaks:

1. **A "Errors" feed in Kibana.** Right now to find a panic you have to
   guess the time range, filter `level:ERROR` by hand, and manually
   correlate to the span that caused it. Kibana's APM UI already renders
   a grouped "top errors" view, stack traces, service health, and
   transaction↔error correlation out of the box. We're paying the
   collection cost anyway; we should get the view for free.
2. **First-class error documents.** `level:ERROR` in `glitch-logs` is a
   free-text message with attrs. APM's error doc schema has `error.id`,
   `error.stack_trace`, `error.exception.type`, `error.grouping_key`,
   and a reference to the owning `trace.id` / `transaction.id`. That's
   the schema you need to ask "has this panic happened before, where,
   and which user?" — a question we currently can't answer without
   grepping stderr.

Secondary: APM gives us a `transaction.duration.histogram` per endpoint
for free, which makes the "why is the brain popover slow for workspace
X" question a single dashboard filter instead of a multi-query
investigation.

## What Changes

- Add **apm-server** to `docker-compose.yml` as a second Elastic service
  alongside the existing `es` + `kibana`. Same network, no external
  ports beyond the OTLP ingest port.
- Point the existing OTel exporter in `internal/telemetry/telemetry.go`
  at the APM server's OTLP endpoint *in addition to* the current direct-
  to-ES custom span exporter. We keep the direct exporter because it's
  cheap, works offline, and survives an APM-server outage; APM becomes
  the primary error UI without becoming a single point of failure.
- Add a **panic → APM error** path in `internal/telemetry`:
  - A `slog.Handler` that, on every `slog.Error` record, also builds an
    APM error doc and ships it to the APM server's `/intake/v2/events`
    NDJSON endpoint (or via OTLP logs, depending on the decision in
    design.md § 3).
  - A helper `telemetry.CaptureError(ctx, err)` that the existing
    `runCollectorGuarded` / pipeline runner / brain panic recover paths
    can call so every recovered panic becomes a queryable error doc
    with the span context attached.
- Wire **service naming** so errors get grouped by service:
  - `gl1tch-cli` for `cmd/serve` and one-shot CLI subcommands
  - `gl1tch-desktop` for the Wails app
  - `gl1tch-pipeline` as a child service for pipeline step executions
    (so "pipeline X is flaky" shows up separately from "the desktop UI
    is crashing")
- Add a `docs/telemetry.md` section documenting the APM UI: how to open
  the Errors view, how service/transaction navigation works, what
  grouping keys we set, and how to correlate an error back to a log
  line in `glitch-logs`.

## Impact

- **Affected specs:** none yet — this is greenfield observability.
  A new `telemetry/apm` capability spec is created under
  `openspec/changes/elastic-apm-error-surfacing/specs/`.
- **Affected code:**
  - `docker-compose.yml` — new `apm-server` service + config
  - `internal/telemetry/telemetry.go` — add OTLP exporter path
    pointed at apm-server
  - `internal/telemetry/apm_error_handler.go` (new) — slog handler that
    forwards ERROR records to APM as error docs
  - `pkg/glitchd/telemetry.go` — no change (the existing wrapper still
    delegates to `telemetry.Setup`)
  - `internal/collector/pod.go`,
    `internal/pipeline/runner.go`,
    `internal/brain/service.go` — call `telemetry.CaptureError` in the
    existing `recover()` blocks so panics become APM error docs
- **Pre-1.0 compatibility:** we wipe and restart on index/schema
  changes, so no migration plumbing. APM's own indices (`apm-*` /
  `traces-apm-*-*`) are managed by apm-server's built-in ILM.

## Out of scope for this change

- Real-user monitoring (apm-rum) — the desktop is a Wails webview but
  tracking frontend performance is a separate conversation.
- Paid-tier APM features (anomaly detection, ML-based alerts). Free
  tier covers the error surfacing we care about.
- Replacing the custom `elasticsearchExporter` in
  `internal/telemetry/elasticsearch_exporter.go`. That stays. The
  custom exporter hits `glitch-traces` (our flat audit index); apm-
  server hits `traces-apm-*` (the schema Kibana APM expects). Two
  destinations, two audiences, both useful.
