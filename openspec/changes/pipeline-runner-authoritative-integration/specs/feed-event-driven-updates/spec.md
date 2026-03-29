## ADDED Requirements

### Requirement: Activity feed seeded from store on Switchboard startup
On Switchboard initialization, the feed SHALL load the most recent N runs from `store.QueryRuns` (default N=50) and populate the ring buffer as `feedEntry` records with `status` set to `Done` or `Failed` based on `Run.ExitStatus`. These entries SHALL be appended before any live busd events are processed, so history is visible immediately after restart.

#### Scenario: Historical runs visible after restart
- **WHEN** Switchboard starts and there are 10 completed runs in the store
- **THEN** the activity feed shows all 10 runs before any new pipeline is launched

#### Scenario: In-flight runs not seeded as done
- **WHEN** a run has `started_at` set but `finished_at` empty (in-flight at restart)
- **THEN** the seeded feed entry shows status `Running` and updates when the run eventually completes

### Requirement: Feed creates or updates entries from pipeline.run.* events
On receiving `pipeline.run.started`, the feed SHALL create a new `feedEntry` with `status: Running`. On `pipeline.run.completed` or `pipeline.run.failed`, the feed SHALL update the corresponding entry's status to `Done` or `Failed` and record `duration_ms`.

#### Scenario: New entry created on run.started
- **WHEN** a `pipeline.run.started` event arrives with `run_id: 42, pipeline: "deploy"`
- **THEN** a new feed entry titled "deploy" appears in the activity feed with running indicator

#### Scenario: Entry updated on run.completed
- **WHEN** a `pipeline.run.completed` event arrives for run_id 42
- **THEN** the corresponding feed entry transitions from Running to Done and stops blinking

#### Scenario: Entry updated on run.failed
- **WHEN** a `pipeline.run.failed` event arrives
- **THEN** the feed entry shows Failed status (red indicator)

### Requirement: Feed updates step state from pipeline.step.* events
On receiving `pipeline.step.started`, `pipeline.step.done`, or `pipeline.step.failed`, the feed SHALL update the `steps` list on the corresponding `feedEntry`. Each `StepInfo` in the entry SHALL track `id`, `status`, and `duration_ms`.

#### Scenario: Step added on step.started
- **WHEN** `pipeline.step.started` arrives for step "build" on run_id 42
- **THEN** the feed entry for run 42 shows "build" in its step list with Running status

#### Scenario: Step updated on step.done
- **WHEN** `pipeline.step.done` arrives for step "build" with `duration_ms: 3200`
- **THEN** the feed entry shows "build" as done with "3.2s" duration

#### Scenario: Step failed shown distinctly
- **WHEN** `pipeline.step.failed` arrives for step "test"
- **THEN** the feed entry shows "test" with Failed status (distinct visual treatment from Done)

### Requirement: Log-line step-status parser retained as fallback only
The existing `[step:<id>] status:<state>` log-line parser in Switchboard SHALL continue to operate but SHALL NOT update a step that already has a terminal status (`done` or `failed`) set via a busd event.

#### Scenario: Log-line does not overwrite busd-set terminal status
- **WHEN** a `pipeline.step.done` busd event has set step "build" to done
- **THEN** a subsequent log line `[step:build] status:done` is silently ignored

#### Scenario: Log-line updates step when no busd event received
- **WHEN** no `pipeline.step.*` busd event arrives but a log line `[step:build] status:running` is parsed
- **THEN** the feed entry updates step "build" to Running status

### Requirement: Feed ring buffer evicts oldest entries at capacity
The feed ring buffer SHALL have a maximum capacity (default 200 entries). When capacity is reached, the oldest `Done` or `Failed` entry SHALL be evicted. `Running` entries SHALL never be evicted.

#### Scenario: Oldest done entry evicted at capacity
- **WHEN** the feed has 200 entries and a new run starts
- **THEN** the oldest non-running entry is removed and the new entry is added

#### Scenario: Running entries are not evicted
- **WHEN** all 200 entries are running
- **THEN** no entry is evicted; a warning is logged; new entries are dropped until capacity frees
