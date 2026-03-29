## ADDED Requirements

### Requirement: Canonical topic constants defined in busd/topics package
The `internal/busd/topics` package SHALL export string constants for all pipeline and cron lifecycle topics:
- `RunStarted = "pipeline.run.started"`
- `RunCompleted = "pipeline.run.completed"`
- `RunFailed = "pipeline.run.failed"`
- `StepStarted = "pipeline.step.started"`
- `StepDone = "pipeline.step.done"`
- `StepFailed = "pipeline.step.failed"`
- `CronJobStarted = "cron.job.started"`
- `CronJobCompleted = "cron.job.completed"`

No other package SHALL use string literals for these topic names.

#### Scenario: Topics package importable without cycle
- **WHEN** `internal/inbox`, `internal/switchboard`, `internal/pipeline`, and `internal/cron` each import `internal/busd/topics`
- **THEN** the build succeeds with no import cycle

#### Scenario: Wildcard subscription matches run events
- **WHEN** a subscriber registers for `"pipeline.run.*"`
- **THEN** it receives events for `pipeline.run.started`, `pipeline.run.completed`, and `pipeline.run.failed`

### Requirement: Runner publishes pipeline.run.started on execution begin
The runner SHALL publish to `pipeline.run.started` before executing any step. The payload SHALL be JSON: `{"run_id": <int64>, "pipeline": "<name>", "started_at": "<RFC3339>"}`.

#### Scenario: run.started published before first step
- **WHEN** a pipeline with one step is executed
- **THEN** `pipeline.run.started` is published before `pipeline.step.started`

#### Scenario: run.started includes run_id from store
- **WHEN** a pipeline run is recorded in the store and then started
- **THEN** the `run_id` in the `pipeline.run.started` payload matches the store run ID

### Requirement: Runner publishes pipeline.run.completed or pipeline.run.failed on finish
On successful completion the runner SHALL publish `pipeline.run.completed`. On failure it SHALL publish `pipeline.run.failed`. Both payloads SHALL include: `{"run_id": <int64>, "pipeline": "<name>", "exit_status": <int>, "duration_ms": <int>, "started_at": "<RFC3339>", "finished_at": "<RFC3339>"}`.

#### Scenario: run.completed published on success
- **WHEN** all steps of a pipeline finish successfully
- **THEN** `pipeline.run.completed` is published with `exit_status: 0`

#### Scenario: run.failed published on step failure
- **WHEN** a step fails and `on_failure` is not `continue`
- **THEN** `pipeline.run.failed` is published with a non-zero `exit_status`

### Requirement: Runner publishes pipeline.step.started, pipeline.step.done, and pipeline.step.failed
For each step, the runner SHALL publish `pipeline.step.started` when execution begins and `pipeline.step.done` or `pipeline.step.failed` when it ends. Payload SHALL include: `{"run_id": <int64>, "pipeline": "<name>", "step": "<id>", "status": "<status>", "duration_ms": <int>}`. Step-done payload SHALL also include `"output": {<truncated map>}` (keys truncated to 512 bytes each, map depth 1).

#### Scenario: step.started published for each step
- **WHEN** a pipeline has three sequential steps
- **THEN** three `pipeline.step.started` events are published in execution order

#### Scenario: step.done includes output map
- **WHEN** step `fetch` completes with output `{"url": "https://example.com", "status": 200}`
- **THEN** `pipeline.step.done` payload contains `"output": {"url": "https://example.com", "status": 200}`

#### Scenario: step.failed published after all retries exhausted
- **WHEN** a step with `retry: 3` fails on all attempts
- **THEN** exactly one `pipeline.step.failed` event is published (not one per retry)

### Requirement: All lifecycle events are published silently on bus unavailability
If the `EventPublisher` returns an error (e.g., bus unreachable), the runner SHALL log the error at debug level and continue execution. No pipeline step SHALL fail due to a publish error.

#### Scenario: Bus error does not abort pipeline
- **WHEN** the publisher returns an error on every call
- **THEN** all pipeline steps execute normally and the run result is unaffected
