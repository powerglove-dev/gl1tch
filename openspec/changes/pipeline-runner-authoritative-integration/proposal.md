## Why

The pipeline runner executes all pipelines and one-off agents but its data — run lifecycle, per-step status, outputs, and timing — is fragmented: the in-memory activity feed in Switchboard diverges from the SQLite store, busd signals are unused for pipeline events, and consumers (inbox, cron, signal board, Go plugins) each work around the missing integration in ad-hoc ways. This change makes the pipeline runner the single authoritative source of truth for all execution state, with busd as the broadcast layer so every part of orcai — and native Go plugins — can react to that truth without duplicating storage or logic.

## What Changes

- **BREAKING**: The pipeline store schema gains a `steps` column (JSON array) so per-step state is persisted alongside the run record; the existing `runs` table is extended, not replaced.
- The runner emits a defined set of busd lifecycle events for run and step transitions: `pipeline.run.started`, `pipeline.run.completed`, `pipeline.run.failed`, `pipeline.step.started`, `pipeline.step.done`, `pipeline.step.failed`, `pipeline.step.output` — each with a canonical JSON payload.
- The `EventPublisher` interface (already specced in `pipeline-event-publisher`) is wired to busd by default in all runner entry points (`cmd/pipeline.go`, `cmd/sysop.go`, cron scheduler).
- The inbox subscribes to `pipeline.run.*` via busd instead of polling the store on a fixed ticker; poll fallback retained for resilience.
- The activity feed and signal board in Switchboard subscribe to `pipeline.run.*` and `pipeline.step.*` events so feed entries reflect real runner state without parsing log lines for step status.
- The cron scheduler emits `cron.job.started` / `cron.job.completed` events and delegates run recording to the pipeline runner (no dual-recording).
- Native Go plugins (Tier 1) receive a `busd.Client` via their execution context so they can publish and subscribe to pipeline events without forking a subprocess.
- The `publish_to` field on steps (already specced in `pipeline-event-publishing`) is fully implemented end-to-end.
- Switchboard's in-memory feed is backed by store query on startup so history survives restarts.
- No code in consumers may directly inspect `feedEntry` state to determine pipeline status — all status flows through busd or the store.

## Capabilities

### New Capabilities

- `pipeline-run-store`: Persistent store for full run records including per-step state (status, output, duration_ms, started_at, finished_at) — extends existing runs table; the canonical data model all consumers read.
- `pipeline-lifecycle-events`: Canonical busd event topics and JSON payloads for all pipeline and step lifecycle transitions; defines the wire contract between the runner and all consumers.
- `pipeline-plugin-bus-access`: Native Go (Tier 1) plugins receive a `busd.Client` injected via execution context so they can subscribe to and publish pipeline events in-process.
- `inbox-event-driven-refresh`: Inbox refreshes on `pipeline.run.*` busd events (replaces fixed 5s poll as primary trigger; poll retained as fallback).
- `feed-event-driven-updates`: Activity feed and signal board entries in Switchboard are driven by busd `pipeline.run.*` and `pipeline.step.*` events; feed is seeded from store on startup.
- `cron-pipeline-delegation`: Cron scheduler delegates run recording entirely to the pipeline runner and emits its own `cron.job.*` lifecycle events rather than duplicating store writes.

### Modified Capabilities

- `pipeline-event-publisher`: Extend payload contract — run-level events add `run_id`, `pipeline`, `started_at`; step-level events add `run_id`, `step`, `status`, `duration_ms`, `output` (truncated).
- `pipeline-event-publishing`: `publish_to` is fully wired to the busd `EventPublisher` — previously specced but unimplemented end-to-end.
- `pipeline-step-lifecycle`: Step execution context snapshot (output map) is persisted to the store after each step completes, not just held in memory.

## Impact

- `internal/store`: schema migration adding `steps` JSONB column and `step_runs` tracking; `RecordStepComplete` added to `StoreWriter` interface.
- `internal/pipeline`: runner wired to busd publisher at all call sites; `ExecutionContext` step outputs written through to store.
- `internal/busd`: no structural changes; new canonical topic constants defined in a `topics` sub-package.
- `internal/inbox`: event subscription added alongside existing store queries.
- `internal/switchboard`: feed seeded from store on init; `feedEntry` status driven by busd events; log-line step-status parsing retained as fallback only.
- `internal/cron`: scheduler wired to pipeline runner's busd publisher; duplicate `RecordRunStart`/`Complete` calls removed.
- `internal/plugin`: `Plugin` interface extended with optional `SetBusClient(busd.Client)` so Tier 1 plugins can opt in to bus access.
- `cmd/pipeline.go`, `cmd/sysop.go`: inject busd publisher into runner options.
- No changes to Tier 2 (sidecar) plugin execution path.
