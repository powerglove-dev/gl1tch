## ADDED Requirements

### Requirement: Feed entries display per-step status badges
Each Activity Feed entry for a pipeline run SHALL show the list of pipeline steps with a live status badge. Status badges SHALL be one of: `pending`, `running`, `done`, `failed`. Steps are displayed in the order they appear in the pipeline YAML.

#### Scenario: Steps shown on entry expansion
- **WHEN** a pipeline feed entry is expanded in the Activity Feed
- **THEN** each step declared in the pipeline is listed with its current status badge

#### Scenario: Running step shows running badge
- **WHEN** the pipeline runner emits a `[step:<id>] status:running` log line
- **THEN** the corresponding step in the feed entry updates its badge to `running`

#### Scenario: Completed step shows done badge
- **WHEN** the pipeline runner emits a `[step:<id>] status:done` log line
- **THEN** the corresponding step in the feed entry updates its badge to `done`

#### Scenario: Failed step shows failed badge
- **WHEN** the pipeline runner emits a `[step:<id>] status:failed` log line
- **THEN** the corresponding step in the feed entry updates its badge to `failed`

#### Scenario: Unknown steps start as pending
- **WHEN** a pipeline run starts and no status line has been received for a step
- **THEN** that step's badge shows `pending`

### Requirement: Pipeline step status is parsed from structured log lines
The Activity Feed log-watcher SHALL parse lines matching the pattern `[step:<id>] status:<state>` and convert them to `StepStatusMsg` values that update the feed entry's per-step state. Lines that do not match the pattern SHALL be treated as plain output lines.

#### Scenario: Status line parsed correctly
- **WHEN** the log-watcher reads the line `[step:fetch] status:running`
- **THEN** a `StepStatusMsg{FeedID: "<entry-id>", StepID: "fetch", Status: "running"}` is sent to the BubbleTea channel

#### Scenario: Non-status line passed through as output
- **WHEN** the log-watcher reads a line that does not start with `[step:`
- **THEN** the line is emitted as a `FeedLineMsg` (existing behaviour) and no `StepStatusMsg` is produced

#### Scenario: Malformed step line ignored
- **WHEN** the log-watcher reads a line like `[step:] status:` (empty id or state)
- **THEN** no `StepStatusMsg` is produced and the line is treated as plain output

### Requirement: Feed entry stores pipeline step list from YAML
When the switchboard starts a pipeline run, it SHALL load the pipeline YAML and populate the feed entry's step list (IDs in declaration order, initial status `pending`). Steps of type `input` and `output` SHALL be omitted from the visible list.

#### Scenario: Step list populated on run start
- **WHEN** the user launches a pipeline named `dual-model-compare`
- **THEN** the new feed entry contains the list of non-input/output step IDs from `dual-model-compare.pipeline.yaml`, each with status `pending`

#### Scenario: Input and output steps excluded
- **WHEN** a pipeline YAML contains steps of type `input` and `output`
- **THEN** those steps do not appear in the feed entry's step list
