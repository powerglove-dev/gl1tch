## ADDED Requirements

### Requirement: User can kill a running pipeline from the signal board
The signal board SHALL allow the user to kill the selected running pipeline entry by pressing `x`. The system SHALL cancel the job's context, kill the associated tmux window (if any), and immediately transition the entry's status to `FeedFailed`. If the selected entry is not in `FeedRunning` state, `x` SHALL be a no-op.

#### Scenario: Kill running pipeline
- **WHEN** the user selects a `FeedRunning` entry in the signal board and presses `x`
- **THEN** the job's cancel function is called, the tmux window is killed, and the entry transitions to `FeedFailed`

#### Scenario: Kill no-op on non-running entry
- **WHEN** the user selects a `FeedDone` or `FeedFailed` entry and presses `x`
- **THEN** no state change occurs and no error is shown

#### Scenario: Kill hint shown only for running entries
- **WHEN** the signal board is focused and the selected entry is `FeedRunning`
- **THEN** the hint bar includes a `x: kill` hint

#### Scenario: Kill hint hidden for non-running entries
- **WHEN** the signal board is focused and the selected entry is not `FeedRunning`
- **THEN** the hint bar does NOT include a kill hint

#### Scenario: Kill with no tmux window
- **WHEN** the user kills a running entry whose `tmuxWindow` field is empty
- **THEN** only the cancel function is called and the entry transitions to `FeedFailed` without error
