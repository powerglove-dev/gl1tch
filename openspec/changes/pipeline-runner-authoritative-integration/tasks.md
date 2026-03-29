## 1. Store: Per-Step Persistence

- [x] 1.1 Add `steps TEXT DEFAULT '[]'` column to `runs` table via schema migration in `internal/store/schema.go`
- [x] 1.2 Define `StepRecord` struct (`id`, `status`, `started_at`, `finished_at`, `duration_ms`, `output`) in `internal/store/store.go`
- [x] 1.3 Add `RecordStepComplete(ctx, runID int64, step StepRecord) error` to the `StoreWriter` interface
- [x] 1.4 Implement `RecordStepComplete` in the SQLite store — JSON upsert by step `id` into the `steps` column
- [x] 1.5 Add `Steps []StepRecord` field to `store.Run` struct and populate it in `QueryRuns` by parsing the `steps` JSON column
- [x] 1.6 Write table-driven tests for `RecordStepComplete` (upsert, unknown run id) and `QueryRuns` step population

## 2. busd Topics Package

- [x] 2.1 Create `internal/busd/topics/topics.go` exporting all canonical topic constants (`RunStarted`, `RunCompleted`, `RunFailed`, `StepStarted`, `StepDone`, `StepFailed`, `CronJobStarted`, `CronJobCompleted`)
- [x] 2.2 Verify zero import cycles: `internal/pipeline`, `internal/inbox`, `internal/switchboard`, `internal/cron` all import `topics` cleanly

## 3. Pipeline Runner: Event Publishing Wired End-to-End

- [x] 3.1 Add `WithEventPublisher(p EventPublisher)` run option to `internal/pipeline/runner.go` (replacing any existing ad-hoc publisher wiring)
- [x] 3.2 Publish `topics.RunStarted` payload (`run_id`, `pipeline`, `started_at`) at the start of `Run` before any step executes
- [x] 3.3 Publish `topics.StepStarted` payload when each step begins execution (both legacy and DAG modes)
- [x] 3.4 Publish `topics.StepDone` or `topics.StepFailed` payload (with `run_id`, `step`, `status`, `duration_ms`, `output`) when each step transitions to terminal state
- [x] 3.5 Call `store.RecordStepComplete` after each step's terminal transition (enqueue asynchronously via existing write queue)
- [x] 3.6 Publish `topics.RunCompleted` or `topics.RunFailed` payload at the end of `Run` after `RecordRunComplete`
- [x] 3.7 Implement `publish_to` step field end-to-end: after step done, marshal output map as JSON and call publisher with the declared topic
- [x] 3.8 Write tests: inject mock `EventPublisher`, verify topic/payload for a 2-step sequential pipeline and a 2-step DAG pipeline

## 4. cmd Layer: busd Publisher Injection

- [x] 4.1 In `cmd/pipeline.go`, attempt to connect to busd using `bus.addr`; wrap as `EventPublisher`; fall back to `NoopPublisher` on failure; pass via `WithEventPublisher`
- [x] 4.2 In `cmd/sysop.go` (agent launch path), apply the same publisher injection
- [ ] 4.3 Verify `orcai pipeline run` emits lifecycle events to a running busd instance (manual smoke test)

## 5. Cron: Remove Duplicate Recording, Add Events

- [x] 5.1 Remove `StoreWriter` dependency from `internal/cron/scheduler.go`; delete the `RecordRunStart`/`RecordRunComplete` calls that duplicate the pipeline runner's recording
- [x] 5.2 Wire busd publisher into the cron scheduler (resolve from `bus.addr` at scheduler init, same fallback pattern as cmd layer)
- [x] 5.3 Publish `topics.CronJobStarted` before subprocess spawn and `topics.CronJobCompleted` after exit, with the canonical payloads
- [x] 5.4 Update cron scheduler tests to assert no store writes and verify the two cron event emissions

## 6. Inbox: Event-Driven Refresh

- [x] 6.1 Subscribe inbox model to `pipeline.run.*` via busd on `Init`; when a `pipeline.run.completed` or `pipeline.run.failed` event arrives, send `RunCompletedMsg{}` into the BubbleTea loop
- [x] 6.2 Extend poll ticker interval to 30 seconds when bus subscription succeeds; keep 5 seconds when bus is unavailable
- [x] 6.3 Update inbox list item rendering to show a step count badge (`"N steps"`) when `len(Run.Steps) > 0`
- [x] 6.4 Write inbox tests: mock busd subscription triggers refresh, fallback poll fires at correct interval

## 7. Switchboard: Feed Seeding and Event-Driven Updates

- [x] 7.1 On Switchboard init, call `store.QueryRuns(50)` and populate the activity feed ring buffer with historical `feedEntry` records (Done/Failed status from `ExitStatus`)
- [x] 7.2 Subscribe Switchboard to `pipeline.run.*` and `pipeline.step.*` busd topics on startup
- [x] 7.3 On `pipeline.run.started`: create new `feedEntry` with Running status keyed by `run_id`
- [x] 7.4 On `pipeline.run.completed`/`pipeline.run.failed`: look up entry by `run_id`, set terminal status and `duration_ms`
- [x] 7.5 On `pipeline.step.started`/`pipeline.step.done`/`pipeline.step.failed`: update the `StepInfo` slice on the corresponding `feedEntry`
- [x] 7.6 Gate the log-line step-status parser: skip update if step already has terminal status set by busd event
- [x] 7.7 Implement ring buffer eviction: at capacity (200), evict oldest Done/Failed entry; never evict Running entries; log warning when all Running
- [x] 7.8 Subscribe Switchboard cron panel to `cron.job.*`; update last-run time and exit-status indicator per entry on `cron.job.completed`

## 8. Plugin Bus Access

- [x] 8.1 Define `BusAwarePlugin` optional interface in `internal/plugin/plugin.go`
- [x] 8.2 In plugin manager, after registering a Tier 1 plugin, check `BusAwarePlugin`; if satisfied, call `SetBusClient` with connected client (or no-op client if bus unavailable)
- [x] 8.3 Implement a `NoopBusClient` in `internal/busd` that satisfies `busd.Client` with silent no-ops
- [x] 8.4 Write a test demonstrating a `BusAwarePlugin` receives a client and can call `Publish` without error when bus is unavailable (no-op path)

## 9. Integration Verification

- [ ] 9.1 Run a multi-step pipeline via `orcai pipeline run` with busd active; confirm store has step records, busd emits all lifecycle events, and feed shows step-level detail
- [ ] 9.2 Restart Switchboard with existing run history in store; confirm activity feed shows historical runs on startup
- [ ] 9.3 Trigger a cron job; confirm single run record in store (no duplicate), `cron.job.*` events visible on bus, cron panel updates
- [x] 9.4 Run `make test` to verify all new and existing tests pass
- [x] 9.5 Fix cron test hang: add `WithExecutable` option to scheduler for test injection
