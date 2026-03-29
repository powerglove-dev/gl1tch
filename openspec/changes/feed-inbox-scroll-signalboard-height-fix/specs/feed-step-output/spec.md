## MODIFIED Requirements

### Requirement: Activity Feed displays per-step output beneath step badge
When a pipeline step completes, the Activity Feed SHALL display the last 5 lines of the step's output beneath the step's status badge. Output SHALL be sourced from the `output.value` field of the `pipeline.step.done` busd event. If the output is empty or absent, no output lines SHALL be shown. The feed's logical-to-visual line map SHALL be recalculated after any output lines are appended so that scroll and cursor bounds remain accurate.

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

#### Scenario: Scroll bounds updated after output appended
- **WHEN** per-step output lines are appended to the feed
- **THEN** the logical-to-visual map is recalculated and the maximum scroll offset reflects the new total line count
