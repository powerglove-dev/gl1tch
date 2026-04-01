## ADDED Requirements

### Requirement: Signal board renders one row per job
The right column SHALL include a SIGNAL BOARD panel above the activity feed. Each job in `m.feed` SHALL correspond to exactly one row in the signal board. The row SHALL display: status LED (●), timestamp, job title, and status label.

#### Scenario: Running job row
- **WHEN** a job has status `FeedRunning`
- **THEN** its signal board row shows a bright `●` LED and the text "running"

#### Scenario: Done job row
- **WHEN** a job has status `FeedDone`
- **THEN** its signal board row shows a green `●` LED and the text "done"

#### Scenario: Failed job row
- **WHEN** a job has status `FeedFailed`
- **THEN** its signal board row shows a red `●` LED and the text "failed"

### Requirement: Running LED blinks
The LED for jobs with `FeedRunning` status SHALL alternate between bright and dim on the existing tick interval, giving a visual blink animation.

#### Scenario: Blink state toggles on tick
- **WHEN** a `tickMsg` is received and a running job exists
- **THEN** the blink state toggles
- **THEN** the next render shows the LED in the opposite brightness

### Requirement: Signal board is keyboard-filterable
When the signal board has focus, pressing `f` SHALL cycle through filter modes: `all` → `running` → `done` → `failed` → `all`. Only rows matching the active filter SHALL be shown. The active filter label SHALL be displayed in the panel header.

#### Scenario: Filter cycles on f
- **WHEN** signal board has focus and user presses `f`
- **THEN** `activeFilter` advances to the next mode
- **THEN** the panel header shows the updated filter name

#### Scenario: Filter hides non-matching rows
- **WHEN** filter is set to "running"
- **THEN** only rows with `FeedRunning` status are rendered

### Requirement: Signal board is part of the focus rotation
Pressing `tab` from the agent runner SHALL move focus to the signal board. Pressing `tab` from the signal board SHALL return focus to the launcher.

#### Scenario: Tab reaches signal board
- **WHEN** agent runner has focus and user presses `tab`
- **THEN** signal board receives focus
- **THEN** a selected row is highlighted
