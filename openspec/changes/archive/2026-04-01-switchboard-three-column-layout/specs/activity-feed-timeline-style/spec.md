## ADDED Requirements

### Requirement: Activity feed is a social-style event timeline
The activity feed SHALL render each agent event as a centered card showing structured metadata only. Raw step output lines (the `step.lines` slice from `StepInfo`) SHALL NOT be rendered in the feed. The feed is a status board, not a log tail.

#### Scenario: No raw output lines appear in the feed
- **WHEN** a feed entry has steps with non-empty `lines` slices
- **THEN** none of those raw lines appear in `viewActivityFeed()` output

#### Scenario: Entry card shows agent name, status, and pipeline name
- **WHEN** a feed entry has an agent name and pipeline name
- **THEN** the card displays agent name, status badge, and pipeline name

### Requirement: Feed cards are horizontally centered
Each entry card SHALL be horizontally centered within the right column. Card width SHALL be `min(rightW - 4, 36)`. The indent SHALL be `max(0, (rightW - cardWidth) / 2)` spaces, computed using visible character width (ANSI stripped).

#### Scenario: Card is indented to center position
- **WHEN** right column inner width is 40 and card width is 36
- **THEN** card is indented by 2 spaces on each side

#### Scenario: Very narrow column does not produce negative indent
- **WHEN** right column is narrower than the minimum card width
- **THEN** indent is 0 and card is rendered flush left inside the border

### Requirement: ANSI box-drawing step connectors
Step names within a card SHALL use Unicode box-drawing connectors:
- Non-final steps: `├─ `
- Final step: `└─ `

#### Scenario: Non-final step uses tee connector
- **WHEN** a step is not the last visible step in an entry
- **THEN** its line starts with `├─ `

#### Scenario: Final step uses corner connector
- **WHEN** a step is the last visible step in an entry
- **THEN** its line starts with `└─ `

### Requirement: 12-hour am/pm timestamps
Feed entry timestamps SHALL be rendered in 12-hour format with lowercase am/pm suffix (e.g., `2:34 pm`). Zero-padding of the hour SHALL NOT be applied.

#### Scenario: AM timestamp formats correctly
- **WHEN** an entry's timestamp is 09:05:00
- **THEN** the rendered timestamp string is `9:05 am`

#### Scenario: PM timestamp formats correctly
- **WHEN** an entry's timestamp is 14:30:00
- **THEN** the rendered timestamp string is `2:30 pm`

#### Scenario: Midnight renders as 12 am
- **WHEN** an entry's timestamp is 00:00:00
- **THEN** the rendered timestamp string is `12:00 am`

#### Scenario: Noon renders as 12 pm
- **WHEN** an entry's timestamp is 12:00:00
- **THEN** the rendered timestamp string is `12:00 pm`
