## ADDED Requirements

### Requirement: OTel traces write to file, not stdout
The telemetry provider SHALL write span data to `$XDG_DATA_HOME/glitch/traces.jsonl` (falling back to `~/.local/share/glitch/traces.jsonl`) instead of stdout. No span JSON SHALL appear on stdout during normal operation.

#### Scenario: Traces file created on first run
- **WHEN** gl1tch starts and executes a pipeline
- **THEN** `~/.local/share/glitch/traces.jsonl` exists and contains newline-delimited JSON span records

#### Scenario: TUI feed contains no raw JSON
- **WHEN** a pipeline runs with tracing enabled
- **THEN** the Activity Feed contains no lines matching `{"Name":` or other raw OTel JSON

### Requirement: Feed entries display per-step duration from spans
Each `StepInfo` entry in a feed item SHALL display a `duration_ms` field populated from the corresponding OTel span's elapsed time. Duration SHALL appear alongside the existing step status badge.

#### Scenario: Duration shown after step completes
- **WHEN** a pipeline step span ends with duration 420ms
- **THEN** the feed entry for that step shows `420ms` adjacent to the step badge

#### Scenario: Duration absent while step is running
- **WHEN** a pipeline step is in-flight
- **THEN** no duration is shown (step is still `FeedRunning`)

#### Scenario: Failed step shows duration
- **WHEN** a pipeline step span ends with ERROR status
- **THEN** the feed entry shows the duration alongside the failure indicator

### Requirement: /trace command renders span tree for selected run
The deck SHALL handle a `/trace` slash command. When invoked with no arguments it SHALL render the span tree for the currently-selected feed entry's run. The span tree SHALL display span name, duration, and status as indented text within the feed detail area.

#### Scenario: /trace with a selected feed entry
- **WHEN** the user types `/trace` with a completed run selected in the feed
- **THEN** the feed detail area shows an indented span tree with name, duration, and OK/ERROR per span

#### Scenario: /trace with no feed entry selected
- **WHEN** the user types `/trace` with no run selected
- **THEN** an inline message reads "no run selected"

#### Scenario: /trace for a run with no recorded spans
- **WHEN** the user types `/trace` for a run that has no spans in traces.jsonl
- **THEN** an inline message reads "no trace data for this run"
