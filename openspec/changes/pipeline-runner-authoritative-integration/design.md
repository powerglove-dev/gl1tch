## Context

ORCAI's pipeline runner already persists run records to SQLite via `store.RecordRunStart`/`RecordRunComplete`, and the `EventPublisher` interface + `NoopPublisher` exist in `internal/pipeline`. However:

- Step-level data (per-step status, output, timing) is only held in-memory in `ExecutionContext` — it disappears when the process exits.
- The activity feed in `Switchboard` is a separate in-memory ring buffer (`feedEntry` structs) that is populated by log-line parsing (`[step:<id>] status:<state>`), not from the authoritative store. On restart, all feed history is lost.
- The inbox polls `store.QueryRuns(50)` on a 5-second ticker — no push mechanism exists.
- `busd` is wired only for `theme.changed` and session events; pipeline lifecycle events are specced but unimplemented end-to-end.
- The cron scheduler independently calls `RecordRunStart`/`RecordRunComplete` on top of the pipeline runner also doing so — duplicate recording when cron invokes `orcai pipeline run`.
- Tier 1 (native Go) plugins have no access to the bus — they can't react to or publish pipeline events.
- The `publish_to` step field is specced (`pipeline-event-publishing`) but the publisher is always `NoopPublisher` at runtime.

The design goal is to make one authoritative flow: **runner → store + busd → all consumers**, with no consumer maintaining shadow state.

## Goals / Non-Goals

**Goals:**
- Per-step state (status, output map, timing) is persisted in the store alongside each run record.
- The runner publishes a canonical set of busd lifecycle events at every run and step transition.
- The busd publisher is injected (not hardcoded) — Noop remains the safe default, real client used when bus is available.
- Inbox, activity feed, and signal board subscribe to busd events as the primary update path; store queries retained as fallback/seed.
- Feed history is seeded from the store on Switchboard startup so restarts don't lose history.
- Cron stops double-recording runs; delegates entirely to the pipeline runner's recording path.
- Tier 1 Go plugins receive an optional `busd.Client` so they can participate in the event bus.
- `publish_to` is fully wired end-to-end.

**Non-Goals:**
- Real-time streaming of step stdout/stderr over busd (too high volume; store + log files remain the source for raw output).
- Changing the Tier 2 (sidecar/CLI adapter) plugin execution path.
- Replacing the SQLite store with a different persistence backend.
- Distributed/multi-node event delivery.
- Changing the `robfig/cron` scheduling foundation.

## Decisions

### D1: Extend `runs` table with a `steps` JSON column rather than a new `step_runs` table

**Decision**: Add a `steps TEXT` column to the existing `runs` table containing a JSON array of step records. Update it incrementally as each step completes via a new `RecordStepComplete(runID, step StepRecord)` method that appends/upserts into the JSON array.

**Rationale**: A separate `step_runs` table adds a join on every inbox/feed query and complicates migration. Steps are always read together with their parent run; JSON column keeps them co-located. SQLite JSON functions allow querying if needed later. The `StoreWriter` interface gets one new method, minimizing consumer impact.

**Alternative considered**: Separate `step_runs` table — rejected due to query complexity and the fact that steps are not queried independently of their run in any current consumer.

### D2: Canonical busd topic namespace under `orcai.pipeline.*`

**Decision**: Define a `topics` sub-package in `internal/busd/topics/` exporting constants:
```
pipeline.run.started
pipeline.run.completed
pipeline.run.failed
pipeline.step.started
pipeline.step.done
pipeline.step.failed
pipeline.step.output      // optional: truncated last N lines of step output
cron.job.started
cron.job.completed
```
Payloads are typed Go structs serialized as JSON. All consumers import `topics` constants — no string literals in subscriber code.

**Rationale**: Centralizing topics prevents typos, enables IDE navigation, and makes the full event contract visible in one file. Wildcard subscriptions (`pipeline.run.*`, `pipeline.step.*`) remain supported by busd's existing pattern matcher.

**Alternative considered**: Embedding constants in the pipeline package — rejected because it creates an import cycle when inbox/switchboard would need to import `pipeline` just for constants.

### D3: busd publisher injected via `RunOption`, resolved at cmd layer

**Decision**: `pipeline.Run(ctx, pl, opts...)` already accepts options. Add `WithEventPublisher(p EventPublisher)`. At `cmd/pipeline.go` and `cmd/sysop.go`, resolve the busd client at startup (read `bus.addr`, attempt connection, fall back to `NoopPublisher` on failure). The cron scheduler similarly resolves the publisher once when the scheduler starts.

**Rationale**: This preserves the existing isolation guarantee (runner package has no import of busd) and keeps the `NoopPublisher` path exercisable in tests without a running bus. No change to the `EventPublisher` interface defined in `pipeline-event-publisher`.

### D4: Inbox uses busd subscription with store-poll fallback

**Decision**: On `Init`, inbox subscribes to `pipeline.run.*` via busd and also performs an initial `store.QueryRuns(50)` to seed the list. On each `pipeline.run.completed` / `pipeline.run.failed` event, inbox re-queries the store (event triggers refresh, store is source of truth for full data). The 5-second poll ticker is retained but interval extended to 30 seconds as a safety net.

**Rationale**: Event-driven refresh eliminates visible lag when a run completes. Store re-query on event (rather than building the run from the event payload alone) avoids duplicating run-rendering logic and ensures step data (from the JSON column) is included. The poll fallback handles the case where the bus is down.

### D5: Activity feed seeded from store; entries driven by busd events

**Decision**: On Switchboard init, load the last N (configurable, default 50) runs from the store and populate the feed ring buffer as `feedEntry` records in `done`/`failed` state. As busd `pipeline.run.*` and `pipeline.step.*` events arrive, create/update entries in real time. The existing log-line step-status parser (`[step:<id>] status:<state>`) is retained as a fallback for externally-launched jobs that don't emit busd events.

**Rationale**: Restart persistence requires the store seed. Real-time updates require busd events. The log-line parser covers the edge case of manually invoked pipelines or plugins that predate this change. Keeping both paths avoids a hard cutover risk.

### D6: Cron delegates recording to pipeline runner; emits own `cron.job.*` events

**Decision**: When the cron scheduler invokes `orcai pipeline run <target>`, it stops calling `RecordRunStart`/`RecordRunComplete` itself. The pipeline runner subprocess handles its own recording. The cron scheduler only emits `cron.job.started` (with `target`, `schedule`, `triggered_at`) and `cron.job.completed` (with `exit_status`, `duration_ms`) events for cron-specific metadata not present in the pipeline run record.

**Rationale**: Eliminates the dual-recording bug (`RecordRunStart` was called twice — confirmed in recent fix commit). Cron-specific metadata (schedule expression, next trigger) is meaningful to a cron monitor panel and doesn't belong in the pipeline run record.

### D7: Tier 1 plugin bus access via optional interface, not required interface change

**Decision**: Define:
```go
type BusAwarePlugin interface {
    Plugin
    SetBusClient(c busd.Client)
}
```
The plugin manager checks `if ba, ok := p.(BusAwarePlugin); ok` after loading a Tier 1 plugin and injects the client. This is a non-breaking opt-in — existing plugins need no changes.

**Rationale**: Making `SetBusClient` part of the base `Plugin` interface would require updating all existing plugins and test doubles. The optional interface pattern (common in Go stdlib, e.g., `http.Flusher`) is idiomatic and keeps the change additive.

## Risks / Trade-offs

- **[Risk] SQLite write contention under high step frequency** → The store already uses a serialized write queue. `RecordStepComplete` appends to a JSON column, which SQLite handles as a full row update. Under very high parallelism (many DAG steps completing concurrently), the queue may add latency. Mitigation: batch step updates within a configurable debounce window (default 200ms); single-step pipelines are unaffected.

- **[Risk] busd unavailable at startup** → All publishers fall back to `NoopPublisher`. The inbox poll fallback (30s) covers this. Consumers that subscribe to busd at init should handle the case where the daemon is not running by attempting reconnect with exponential backoff rather than crashing.

- **[Risk] Feed seeding on startup scans up to 50 store rows** → Negligible for SQLite at this scale. The query is already used by inbox today.

- **[Risk] JSON step column grows large for long-running DAG pipelines** → Each step record is O(1KB). A 50-step pipeline produces ~50KB per run. Auto-prune (already implemented) limits total rows. No action needed unless pipelines regularly exceed hundreds of steps.

- **[Risk] Log-line step-status parser and busd events produce duplicate feed updates** → Resolved by checking: if a `feedEntry` already has a step in `done`/`failed` state from a busd event, ignore subsequent log-line updates for that step. Log-line parser is demoted to fallback-only, not primary path.

## Migration Plan

1. **Store migration** (additive): Add `steps TEXT DEFAULT '[]'` column to `runs` table. SQLite `ALTER TABLE` supports this. Existing rows get empty arrays — no data loss. Migration runs on first startup via the existing schema upgrade path.
2. **Publisher wiring** (additive): `WithEventPublisher` option is additive — existing call sites that don't pass it get `NoopPublisher`. Wire in `cmd/pipeline.go` and `cmd/sysop.go` in a single commit.
3. **Cron de-duplication** (breaking fix): Remove duplicate `RecordRunStart`/`RecordRunComplete` calls from cron scheduler. This is already identified as a bug (fix commit `5c90fd9`). Validate by running a cron-triggered pipeline and confirming single run record.
4. **Inbox / feed subscriptions** (additive): Subscribe to busd events alongside existing poll/log paths. Validate that inbox shows runs immediately on completion before removing/extending the poll interval.
5. **Rollback**: All changes are additive except the cron de-duplication. The `steps` column has a default and is ignored by existing query code. Rolling back cron de-duplication restores the duplicate-recording behavior (no data loss, just duplicates).

## Open Questions

- **Q1**: Should `pipeline.step.output` events be emitted (last N lines of step stdout)? This enables real-time output streaming to the feed but adds busd volume. Decision deferred — implement as opt-in via a runner option flag.
- **Q2**: For the inbox panel, should busd subscription drive a `tea.Cmd` push directly into the BubbleTea message loop, or should the subscription set a dirty flag that the poll ticker checks? Push is more responsive but requires the subscription goroutine to hold a reference to the program's `Send` func. Recommendation: use `program.Send(RunCompletedMsg{})` pattern already established in the codebase.
- **Q3**: How many historical runs should the feed seed on startup? Default 50 matches inbox. Should be configurable via the same retention config that governs auto-prune.
