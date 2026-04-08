## ADDED Requirements

### Requirement: Explicit thumbs feedback on assistant answers
Every assistant answer SHALL render a thumbs-up and thumbs-down control. Clicking either SHALL emit a `brain_feedback` event with the answer ID and the verdict.

#### Scenario: Thumbs-up emits an event
- **WHEN** the user clicks thumbs-up on an assistant answer
- **THEN** a `brain_feedback` event SHALL be written with `verdict=accepted, source=explicit, answer_id=…`

#### Scenario: Thumbs-down emits an event
- **WHEN** the user clicks thumbs-down on an assistant answer
- **THEN** a `brain_feedback` event SHALL be written with `verdict=rejected, source=explicit, answer_id=…`

### Requirement: Implicit feedback from contradicting follow-up
When a user runs another `glitch ask` within 5 minutes of an assistant answer, the system SHALL classify the follow-up's intent against the prior answer using a single qwen2.5:7b call and SHALL emit a `brain_feedback` event when the intent is `rephrase`, `challenge`, `correct`, or `ignore_previous`.

#### Scenario: Contradicting follow-up emits a not-accepted event
- **WHEN** within 5 minutes of an answer the user runs a follow-up classified as `correct`
- **THEN** a `brain_feedback` event SHALL be written with `verdict=not_accepted, source=implicit, classification=correct`

#### Scenario: Unrelated follow-up emits no event
- **WHEN** within 5 minutes of an answer the user runs a follow-up classified as `unrelated`
- **THEN** no `brain_feedback` event SHALL be written for the prior answer

### Requirement: Feedback events carry config hash
Each `brain_feedback` event SHALL embed the brain/research-loop config hash that was active when the original answer was produced, so that the stats engine can attribute outcomes to a specific configuration.

#### Scenario: Config hash present on event
- **WHEN** any `brain_feedback` event is emitted
- **THEN** it SHALL include a `config_hash` field referencing the configuration in effect when the answer was produced

### Requirement: Implicit classifier verdict is logged
The implicit classifier's raw verdict SHALL be written to the event store alongside the resulting `brain_feedback` event so that the classifier itself can be audited and re-classified later.

#### Scenario: Classifier verdict stored
- **WHEN** the implicit feedback path runs
- **THEN** the classifier's full verdict (label + raw model output) SHALL be persisted, not just the boolean outcome
