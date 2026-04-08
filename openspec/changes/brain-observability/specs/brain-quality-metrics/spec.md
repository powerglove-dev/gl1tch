## ADDED Requirements

### Requirement: Five defined brain quality metrics
The system SHALL define exactly five brain quality metrics: `accept_rate`, `confidence_calibration`, `retrieval_precision`, `iteration_count`, and `escalation_rate`. Each metric SHALL have a documented formula and a documented "smarter" direction.

#### Scenario: Each metric has a defined formula
- **WHEN** a developer reads the metric definitions
- **THEN** they SHALL find for each metric: an exact formula in terms of brain event fields, the event types it consumes, and whether higher or lower values mean the brain is improving

### Requirement: accept_rate computation
`accept_rate` SHALL be the fraction of assistant answers in the window that were accepted, where "accepted" means an explicit thumbs-up *or* the absence of an implicit not-accepted signal within 5 minutes.

#### Scenario: Explicit thumbs-up counts as accept
- **WHEN** a user clicks thumbs-up on an assistant answer
- **THEN** that answer SHALL be counted as accepted in `accept_rate`

#### Scenario: Contradicting follow-up counts as not-accepted
- **WHEN** within 5 minutes of an assistant answer the user runs another `glitch ask` whose intent classifier labels the follow-up as `rephrase`, `challenge`, `correct`, or `ignore_previous`
- **THEN** the original answer SHALL be counted as not-accepted in `accept_rate`

#### Scenario: No signal defaults to accepted
- **WHEN** an assistant answer receives no explicit feedback and no contradicting follow-up within 5 minutes
- **THEN** it SHALL be counted as accepted in `accept_rate`

### Requirement: confidence_calibration is the Brier score
`confidence_calibration` SHALL be computed as the mean squared error between the stated composite confidence on each accepted answer and the binary accept outcome (1 = accepted, 0 = not).

#### Scenario: Perfect calibration scores 0
- **WHEN** every answer with stated confidence 1.0 was accepted and every answer with stated confidence 0.0 was not
- **THEN** the Brier score SHALL be 0

#### Scenario: Lower Brier is better
- **WHEN** the rendering layer displays `confidence_calibration`
- **THEN** it SHALL accompany the value with a one-line interpretation indicating that lower values mean better calibration

### Requirement: retrieval_precision computation
`retrieval_precision` SHALL be the fraction of researcher evidence items in the bundle that the critique pass labeled as `cited` in the accepted draft.

#### Scenario: All evidence cited
- **WHEN** every item in the bundle is cited in the final draft
- **THEN** `retrieval_precision` for that answer SHALL be 1.0

### Requirement: iteration_count computation
`iteration_count` SHALL be the average number of research-loop iterations per accepted answer in the window.

#### Scenario: Single-iteration accepts contribute 1
- **WHEN** an answer was accepted on the first iteration
- **THEN** it SHALL contribute the value 1 to the average

### Requirement: escalation_rate computation
`escalation_rate` SHALL be the fraction of accepted answers in the window that involved a paid-model escalation.

#### Scenario: No escalation contributes 0
- **WHEN** an accepted answer's research trace contains no `research_escalation` event
- **THEN** it SHALL contribute 0 to the escalation rate

### Requirement: Metric direction documented
Each metric SHALL document its "smarter" direction so that the rendering layer can show trend arrows correctly.

#### Scenario: Direction guides trend rendering
- **WHEN** the `score_card` renderer displays a metric's sparkline
- **THEN** the trend arrow SHALL be derived from the documented direction (up = better for `accept_rate`, down = better for `confidence_calibration`, `iteration_count`, `escalation_rate`; up = better for `retrieval_precision`)
