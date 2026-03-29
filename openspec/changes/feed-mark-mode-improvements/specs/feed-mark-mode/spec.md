## ADDED Requirements

### Requirement: Activity Feed mark mode cycles through active and paused states
The Activity Feed SHALL maintain a mark mode state that cycles between off, active, and paused when the user presses `m`. When mark mode is off, pressing `m` SHALL enter active mark mode. When mark mode is active, pressing `m` SHALL pause mark mode. When mark mode is paused, pressing `m` SHALL resume active mark mode. The current mark mode SHALL be indicated in the hint bar.

#### Scenario: Pressing m from normal mode enters active mark mode
- **WHEN** mark mode is off and the user presses `m`
- **THEN** the feed enters active mark mode and the hint bar reflects the active state

#### Scenario: Pressing m from active mark mode pauses it
- **WHEN** mark mode is active and the user presses `m`
- **THEN** mark mode transitions to paused

#### Scenario: Pressing m from paused mark mode resumes active mark mode
- **WHEN** mark mode is paused and the user presses `m`
- **THEN** mark mode transitions back to active

### Requirement: j/k in active mark mode marks/unmarks lines while navigating
When mark mode is active, pressing `j` or `k` SHALL mark or unmark the line at the current cursor position, then move the cursor down or up respectively. If the current line is already marked, navigating SHALL unmark it. If it is unmarked, navigating SHALL mark it.

#### Scenario: j marks the current line and moves down in active mark mode
- **WHEN** mark mode is active and the user presses `j`
- **THEN** the current cursor line is toggled (marked if unmarked, unmarked if marked) and the cursor moves down one line

#### Scenario: k marks the current line and moves up in active mark mode
- **WHEN** mark mode is active and the user presses `k`
- **THEN** the current cursor line is toggled and the cursor moves up one line

### Requirement: j/k in paused mark mode navigates without marking
When mark mode is paused, pressing `j` or `k` SHALL move the cursor without toggling any marks.

#### Scenario: j moves cursor without marking in paused mark mode
- **WHEN** mark mode is paused and the user presses `j`
- **THEN** the cursor moves down one line and no mark is toggled

#### Scenario: k moves cursor without marking in paused mark mode
- **WHEN** mark mode is paused and the user presses `k`
- **THEN** the cursor moves up one line and no mark is toggled

### Requirement: Mark mode resets to off when the feed loses focus
When the feed panel loses focus, mark mode SHALL reset to off. Existing marked lines are cleared as part of the reset.

#### Scenario: Switching panels clears mark mode
- **WHEN** mark mode is active and the user switches focus away from the feed
- **THEN** mark mode returns to off and all marks are cleared
