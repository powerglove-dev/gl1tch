# Tasks — Elastic APM error surfacing

Sequenced so each step is independently testable and the system stays
usable if the chain is interrupted.

## Phase 1 — apm-server deployment

- [x] Add `apm-server` service to `docker-compose.yml`
      (anonymous auth, port 8200, pinned to the same Elastic stack
      version as `es`/`kibana`).
- [x] Bring up the stack, curl `http://localhost:8200/` and confirm
      the apm-server health endpoint responds. (Returns 200 with
      `publish_ready: true`, OTLP paths `/v1/traces`, `/v1/metrics`,
      `/v1/logs` registered.)
- [x] Added `services:up` / `services:down` / `services:status` /
      `services:logs` / `services:nuke` tasks in `Taskfile.yml` so
      the stack is one-command up/down/check.
- [x] Kibana APM Services view renders the `gl1tch-desktop` service
      with its transaction count + span histogram populated by the
      OTLP exporter.

## Phase 2 — OTLP exporter wiring

- [x] In `internal/telemetry/telemetry.go`, added an
      `otlptracegrpc` exporter batcher gated on
      `GL1TCH_APM_DISABLE=1` (opt-out) with default endpoint
      `localhost:8200`. `GL1TCH_APM_ENDPOINT` overrides.
- [x] Preserved the existing `elasticsearchExporter` unchanged —
      both batchers run in parallel, verified by identical counts
      in `glitch-traces` and `.ds-traces-apm-default-*` (37/37 from
      the verification run).
- [x] Also moved the ES address resolution out of
      `collector.LoadConfig` into a `GL1TCH_ES_ADDRESS` env lookup
      so `internal/telemetry` no longer imports `internal/collector`
      — otherwise the `internal/collector → telemetry.CaptureError`
      edge would have created an import cycle.
- [ ] In-memory OTLP receiver integration test still pending —
      real apm-server verification covers the wire path, but a
      unit test would catch schema drift earlier. Tracked but
      deferred.

## Phase 3 — Service naming

- [x] `glitch-desktop/app.go` startup passes `gl1tch-desktop` to
      `glitchd.SetupTelemetry` (already in place, confirmed).
- [x] `main.go` updated to pass `gl1tch-cli` (was `gl1tch`).
- [x] Pipeline subprocess service name: revised approach. Pipelines
      run in-process inside the parent binary (CLI or desktop), so
      we don't spawn a second `telemetry.Setup`. Instead, the
      existing `otel.Tracer("gl1tch/pipeline")` scope name in
      `pipeline/runner.go` gives us the same per-subsystem grouping
      in APM's Transactions view (`scope.name: gl1tch/pipeline`)
      without running a separate service. Documented in design.md
      § Decisions/Service naming and left as-is.
- [x] Verified `gl1tch-desktop` appears in the APM Services view
      during end-to-end verification. `gl1tch-cli` will appear when
      the CLI runs under the new build.

## Phase 4 — Error capture

- [x] Added `internal/telemetry/apm_error.go`:
      - `apmErrorSink` type + `ErrorSink` interface + package-level
        `ActiveErrorSink()` getter (so logbuffer can route records
        without importing the concrete type)
      - NDJSON POST to `/intake/v2/events`, rate-limited stderr
        reporting on failures (1/30s) to prevent error loops
      - sha1 grouping key on (service.name, culprit, top frame)
      - Exception type extraction from culprit prefix
        ("git collector" from "git collector: poll error")
      - Stack capture via `runtime.Callers` with runtime-frame
        skipping
      - Trace/transaction ID pull from
        `trace.SpanContextFromContext(ctx)`
- [x] Extended `pkg/glitchd/logbuffer.go` `esTeeHandler.Handle` so
      `level >= LevelError` records call `telemetry.CaptureLog`
      alongside the existing ES enqueue. No-op when sink is nil
      (APM disabled or apm-server unreachable at startup).
- [x] Added package-level `telemetry.CaptureError(ctx, err, attrs,
      stackSkip)` + `telemetry.CaptureLog(ctx, msg, attrs)` so
      call sites don't need to know about `ActiveErrorSink()`.
- [x] Wired `telemetry.CaptureError` into recover blocks:
      - `internal/collector/pod.go` `runCollectorGuarded` — passes
        ctx so collector.run trace is correlated
      - `internal/pipeline/runner.go` `launchStep` goroutine — NEW
        recover block (previously unprotected); sends a synthetic
        `stepResult{err}` down `completedCh` so the dispatcher
        moves on to `on_failure`
      - `internal/brain/service.go` `cycle` — NEW recover block
        (previously unprotected); next tick retries cleanly
- [x] Ran `rg "recover\(\)" internal/` — only the three locations
      above plus test code.
- [ ] Unit test for the NDJSON body shape — covered in practice by
      the end-to-end Python curl test (status 202 from apm-server
      for the exact Go-produced schema) but a Go unit test against
      `httptest.NewServer` is still a good idea. Deferred.

## Phase 5 — Verification

- [x] End-to-end desktop run for ~60s with ES+Kibana+apm-server up:
      | index | count |
      |---|---|
      | `glitch-logs` | 1814 |
      | `glitch-traces` | 37 |
      | `.ds-traces-apm-default-*` | 37 (exact parity with above) |
      | `.ds-logs-apm.error-default-*` | 3 (1 natural + 2 smoketest) |

      APM decomposition matches intent:
      - Transactions: `workspace.pod.start`, `brain.cycle`
      - Spans: `collector.poll` × 34 — **per-tick spans export on
        the normal batch schedule, not waiting for shutdown**
      - Services: `gl1tch-desktop` (real) + `gl1tch-apmsmoke`
        (throwaway test binary, cleaned up after)
- [x] End-to-end smoke test via `cmd/apmsmoke` (now deleted):
      - `slog.Error` → `apmErrorSink.CaptureLog` → APM → grouped
        error with no stack (correct — plain logs have no stack)
      - `recover()` → `slog.Error` with panic attr → APM → grouped
        error — verified two distinct culprits land as two
        distinct groups
- [x] Python `/intake/v2/events` POST with the exact Go schema
      returned 202, confirming wire-level schema compatibility.
- [ ] apm-server down test — not yet exercised. Verify behavior
      when apm-server is unavailable: (a) error rate-limit line
      appears on stderr once per 30s, (b) everything else stays
      responsive. Deferred to a follow-up manual test.

## Phase 6 — Documentation

- [ ] Update `docs/telemetry.md`:
      - What services are named and why
      - How to find an error in Kibana APM
      - How to correlate an APM error back to `glitch-logs`
      - How to disable APM shipping
        (`GL1TCH_APM_ENDPOINT=` unset OR a new
        `GL1TCH_APM_DISABLE=1`?)
- [ ] Note in `docs/telemetry.md` that error shipping is from the
      `slog.Error` + `recover()` paths only, and that plain
      `fmt.Fprintln(os.Stderr, ...)` and `log.Printf` calls do NOT
      become APM errors — they land only in the terminal.

## Phase 7 — Per-tick child spans

User explicitly asked for this in the same message as "implement",
so it landed in the same change instead of a follow-up.

- [x] Added `internal/collector/tick_span.go` with
      `startTickSpan(ctx, collector, workspaceID) (ctx, done)`.
      The `done` closure records `indexed`, `duration_ms`, and
      status (OK / Error + error attachment), then calls
      `span.End()` so the BatchSpanProcessor exports on the normal
      5s schedule — no shutdown drain required.
- [x] Wired `startTickSpan` into every collector's tick loop:
      git, github, directory, claude, claude-projects, code-index,
      pipeline, copilot. (mattermost was subsequently removed from
      the codebase entirely — see the follow-up "drop mattermost"
      commit.)
- [x] `code_index.CodeIndexCollector.runOnce` signature bumped to
      `(int, error)` return so the per-tick `done` closure gets
      the real chunk count and last error.
- [x] All inner ES + subprocess calls in each tick now take
      `tickCtx` instead of the outer collector context, so nested
      spans (BulkIndex, HTTP requests, etc.) become grandchildren
      of `collector.poll` and show up in APM's waterfall view.
- [x] Verified: 34 `collector.poll` span docs landed in
      `.ds-traces-apm-default-*` from a live desktop run (not from
      shutdown drain) within 60s of startup.

Deferred further refinements to a future change:
- `collector.scan` child spans for directory-walking collectors
  (one per directory instead of one per tick)
- `collector.index` child spans wrapping each `es.BulkIndex` call
  so index-latency histograms split out from poll latency
- Per-collector span links to the parent `collector.run` span
  (currently they inherit via ctx parent-child, which is the
  standard pattern)

## Out of scope (explicitly)

- apm-rum (frontend RUM tracking for the Wails webview)
- ML-based anomaly detection / alerting
- Replacing `glitch-logs` with OTLP logs shipped to APM — deferred
  (see design.md § Open questions #1)
- Parent-based sampling — deferred until span volume becomes a
  problem (see design.md § Open questions #3)
