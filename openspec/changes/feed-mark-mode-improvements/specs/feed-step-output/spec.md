## MODIFIED Requirements

### Requirement: Activity Feed displays per-step output beneath step badge
When a pipeline step completes, the Activity Feed SHALL display the last 5 lines of the step's output beneath the step's status badge. Output SHALL be sourced from the `output.value` field of the `pipeline.step.done` busd event. If the output is empty or absent, no output lines SHALL be shown. Steps with status `done` and no output lines SHALL be omitted from the feed render entirely — their badge SHALL NOT be displayed. Steps with status `running`, `failed`, or `pending` SHALL always display their badge regardless of output.

#### Scenario: Step output appears after step completes
- **WHEN** a pipeline step emits a `pipeline.step.done` event with `output.value: "line1\nline2\nline3"`
- **THEN** the Activity Feed shows three indented lines beneath the step badge

#### Scenario: Long output is truncated to last 5 lines
- **WHEN** a step's output contains 10 lines
- **THEN** only the last 5 lines are shown beneath the step badge

#### Scenario: Done step with no output is suppressed
- **WHEN** a step's status is `done` and its output is empty or absent
- **THEN** no badge or output lines appear for that step in the feed

#### Scenario: Running step with no output still shows its badge
- **WHEN** a step's status is `running` and it has produced no output yet
- **THEN** the step badge is rendered in the feed

#### Scenario: Failed step with no output still shows its badge
- **WHEN** a step's status is `failed` and it has no output
- **THEN** the step badge is rendered in the feed

#### Scenario: Step output survives scroll
- **WHEN** the user scrolls up in the Activity Feed
- **THEN** per-step output lines scroll together with their parent step badge

## ADDED Requirements

### Requirement: Activity Feed renders steps with tree connectors
Steps within a feed entry SHALL be rendered using tree connectors to show hierarchy. All steps except the last visible step SHALL use `├` as their connector. The last visible step SHALL use `└`. Output lines beneath a step SHALL be indented under the step's connector using `│` to maintain the vertical tree line, except beneath the last step where no continuation line is needed.

#### Scenario: Non-final steps render with fork connector
- **WHEN** a feed entry has multiple steps and the current step is not the last visible step
- **THEN** the step badge is prefixed with `├ ` (fork connector)

#### Scenario: Final step renders with end connector
- **WHEN** a feed entry has multiple steps and the current step is the last visible step
- **THEN** the step badge is prefixed with `└ ` (end connector)

#### Scenario: Output beneath non-final step uses vertical connector
- **WHEN** a non-final step has output lines
- **THEN** each output line is prefixed with `│ ` to continue the tree

#### Scenario: Output beneath final step uses plain indent
- **WHEN** the final step has output lines
- **THEN** each output line is indented with spaces (no vertical connector)
