## ADDED Requirements

### Requirement: Inbox subscribes to pipeline.run.* events for push-driven refresh
The inbox model SHALL subscribe to the `pipeline.run.*` busd topic on initialization. On receiving any `pipeline.run.completed` or `pipeline.run.failed` event, the inbox SHALL trigger a store refresh immediately (re-query `store.QueryRuns`). This is the primary refresh path; the poll ticker is retained as a fallback.

#### Scenario: Inbox refreshes immediately on run.completed
- **WHEN** a `pipeline.run.completed` event is published to the bus
- **THEN** the inbox updates its list within one render cycle, without waiting for the next poll tick

#### Scenario: Inbox refreshes immediately on run.failed
- **WHEN** a `pipeline.run.failed` event is published to the bus
- **THEN** the inbox list shows the failed run with the correct status on next render

#### Scenario: Inbox functions normally when bus is unavailable
- **WHEN** the busd daemon is not running
- **THEN** the inbox falls back to store polling (extended interval) and renders existing runs without error

### Requirement: Inbox poll interval extended to 30 seconds when bus is available
When the inbox successfully subscribes to the bus, the poll ticker interval SHALL be extended from 5 seconds to 30 seconds. When the bus is unavailable, the original 5-second interval SHALL be used.

#### Scenario: Poll interval is 30s with bus connected
- **WHEN** the inbox successfully connects to busd on init
- **THEN** the background poll fires every 30 seconds

#### Scenario: Poll interval is 5s without bus
- **WHEN** busd is unavailable on inbox init
- **THEN** the background poll fires every 5 seconds

### Requirement: Inbox initial load includes step data from store
When the inbox loads a run from `store.QueryRuns`, the run's `Steps` slice SHALL be available for rendering. The inbox view for a run SHALL display a step count badge (e.g., "3 steps") when `len(Run.Steps) > 0`.

#### Scenario: Step count shown for multi-step run
- **WHEN** a run with 4 completed steps is displayed in the inbox
- **THEN** the inbox list item shows "4 steps" or equivalent step count indicator

#### Scenario: No step badge for single-command runs
- **WHEN** a run has 0 steps (e.g., a raw subprocess job)
- **THEN** no step count badge is displayed
