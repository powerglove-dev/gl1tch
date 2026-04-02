## MODIFIED Requirements

### Requirement: Activity Feed displays per-step output and timing beneath step badge
When a pipeline step completes, the Activity Feed SHALL display the last 5 lines of the step's output beneath the step's status badge. Output SHALL be sourced from the `output.value` field of the `pipeline.step.done` busd event. If the output is empty or absent, no output lines SHALL be shown. The feed SHALL also display the step's `duration_ms` sourced from the corresponding OTel span alongside the step badge.

#### Scenario: Step output appears after step completes
- **WHEN** a pipeline step emits a `pipeline.step.done` event with `output.value: "line1\nline2\nline3"`
- **THEN** the Activity Feed shows three indented lines beneath the step badge

#### Scenario: Long output is truncated to last 5 lines
- **WHEN** a step's output contains 10 lines
- **THEN** only the last 5 lines are shown beneath the step badge

#### Scenario: Empty output shows no lines
- **WHEN** a step's `output.value` is empty or the output map has no value key
- **THEN** no output lines appear beneath the step badge

#### Scenario: Step output survives scroll
- **WHEN** the user scrolls up in the Activity Feed
- **THEN** per-step output lines scroll together with their parent step badge

#### Scenario: Step duration shown after span ends
- **WHEN** a pipeline step span ends with duration 210ms
- **THEN** the step badge in the feed entry shows `210ms` alongside the step name
