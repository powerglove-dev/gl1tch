## ADDED Requirements

### Requirement: Inbox detail displays per-step output beneath step badge
When rendering the steps section in the inbox detail view, each step that has a non-empty `Output["value"]` SHALL display up to the last 5 lines of that output indented beneath the step badge line. Steps with empty or absent output SHALL show no output lines.

#### Scenario: Step output appears beneath step badge
- **WHEN** a step has `Output["value"]` containing one or more lines
- **THEN** the inbox detail renders those lines (up to 5, last 5 if more) indented beneath the step badge

#### Scenario: Long step output is truncated to last 5 lines
- **WHEN** a step's output contains more than 5 lines
- **THEN** only the last 5 lines are shown beneath the step badge in the inbox detail

#### Scenario: Empty output shows no lines
- **WHEN** a step's `Output["value"]` is empty or the output map has no value key
- **THEN** no output lines appear beneath the step badge in the inbox detail

#### Scenario: Step output scrolls with parent badge
- **WHEN** the user scrolls the inbox detail view
- **THEN** step output lines scroll together with their parent step badge, as they are part of the same content block

#### Scenario: Output lines use dim styling
- **WHEN** step output lines are rendered
- **THEN** they are styled using the palette's dim color, visually subordinate to the step badge line
