## ADDED Requirements

### Requirement: Cron scheduler does not record runs in the store directly
The cron scheduler SHALL NOT call `RecordRunStart` or `RecordRunComplete` on the store when invoking a pipeline via `orcai pipeline run`. Run recording is the responsibility of the pipeline runner subprocess. The cron scheduler's `StoreWriter` dependency SHALL be removed or limited to cron-specific metadata only.

#### Scenario: Single run record created per cron-triggered pipeline
- **WHEN** cron triggers a pipeline and the pipeline runner executes
- **THEN** exactly one run record exists in the store for that invocation (not two)

#### Scenario: Cron scheduler starts without store dependency
- **WHEN** the cron scheduler is initialized without a StoreWriter
- **THEN** it loads and schedules entries normally; no store-related error occurs

### Requirement: Cron scheduler publishes cron.job.started and cron.job.completed events
When a cron entry fires, the scheduler SHALL publish `cron.job.started` before invoking the subprocess and `cron.job.completed` after it exits. Payload for `started`: `{"job": "<name>", "target": "<pipeline>", "schedule": "<cron-expr>", "triggered_at": "<RFC3339>"}`. Payload for `completed`: `{"job": "<name>", "target": "<pipeline>", "exit_status": <int>, "duration_ms": <int>, "finished_at": "<RFC3339>"}`.

#### Scenario: cron.job.started published before subprocess launch
- **WHEN** a cron entry for pipeline "daily-sync" fires
- **THEN** `cron.job.started` is published before the subprocess is spawned

#### Scenario: cron.job.completed published after subprocess exits
- **WHEN** the subprocess for "daily-sync" exits with code 0
- **THEN** `cron.job.completed` is published with `exit_status: 0` and a positive `duration_ms`

#### Scenario: cron.job.completed published even on subprocess failure
- **WHEN** the subprocess exits with a non-zero exit code
- **THEN** `cron.job.completed` is published with the actual `exit_status`

### Requirement: Cron panel in Switchboard reacts to cron.job.* events
The Switchboard cron panel SHALL subscribe to `cron.job.*` events and update the last-run timestamp and last-exit-status for each entry when events are received. This replaces any polling of the store for cron-specific run history.

#### Scenario: Last run time updated on cron.job.completed
- **WHEN** `cron.job.completed` arrives for job "daily-sync"
- **THEN** the cron panel displays the `finished_at` timestamp as the last run time for "daily-sync"

#### Scenario: Failed job shown with error indicator
- **WHEN** `cron.job.completed` arrives with `exit_status: 1`
- **THEN** the cron panel shows an error indicator next to the "daily-sync" entry
