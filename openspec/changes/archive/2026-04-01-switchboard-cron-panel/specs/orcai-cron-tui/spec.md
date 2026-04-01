## ADDED Requirements

### Requirement: orcai-cron TUI runs as a BubbleTea application
The `orcai cron tui` subcommand SHALL launch a full-screen BubbleTea TUI that embeds the cron scheduler in-process. The TUI SHALL replace the bare `orcai cron run` daemon as the default session command launched by `orcai cron start`.

#### Scenario: TUI starts and shows job list
- **WHEN** the user runs `orcai cron tui`
- **THEN** a full-screen BubbleTea TUI is displayed with a top pane (job list) and bottom pane (log viewer)

#### Scenario: cron start launches TUI
- **WHEN** the user runs `orcai cron start`
- **THEN** the created tmux session runs `orcai cron tui` instead of `orcai cron run`

### Requirement: Top pane shows filterable job list
The top pane SHALL display all cron entries with columns: name, kind, schedule expression, next-run time (relative), and last-run status. The user SHALL be able to type to activate fuzzy search, filtering the list by job name.

#### Scenario: Job list displays all entries
- **WHEN** the TUI is open and `cron.yaml` has entries
- **THEN** each entry appears as a row showing name, kind, schedule, and next-run time

#### Scenario: Fuzzy search filters job list
- **WHEN** the user types characters in the search input
- **THEN** the job list is filtered to entries whose names fuzzy-match the input

#### Scenario: Empty search shows full list
- **WHEN** the search input is cleared
- **THEN** all entries are shown unfiltered

#### Scenario: Empty state message
- **WHEN** `cron.yaml` has no entries
- **THEN** the top pane shows a placeholder (e.g. "no scheduled jobs — add one from the switchboard")

### Requirement: Bottom pane shows live structured log output
The bottom pane SHALL display a scrolling log stream using `charmbracelet/log`-formatted output. Each log line SHALL be prefixed with the cron job name as a structured field so operators can identify which job produced each line.

#### Scenario: Log lines are prefixed with job name
- **WHEN** a cron job runs and produces log output
- **THEN** each log line in the bottom pane includes the job name as a prefix field (e.g. `INFO job=my-pipeline pipeline finished`)

#### Scenario: Log viewer scrolls automatically
- **WHEN** new log lines arrive
- **THEN** the bottom pane scrolls to show the most recent output

#### Scenario: Log buffer is bounded
- **WHEN** the log output exceeds 500 lines
- **THEN** the oldest lines are discarded to keep the buffer at 500 lines

### Requirement: Edit action opens a form overlay for selected job
Pressing `e` on a selected job SHALL open an inline form overlay pre-populated with the entry's fields (name, schedule, kind, target, timeout). Confirming with `enter` SHALL save the changes to `cron.yaml` and trigger a scheduler reload. Pressing `esc` SHALL cancel without changes.

#### Scenario: Edit overlay pre-populates fields
- **WHEN** the user presses `e` on a selected entry
- **THEN** a form overlay appears with name, schedule, kind, target, and timeout fields populated from the entry

#### Scenario: Confirm saves and reloads
- **WHEN** the user modifies fields and presses `enter`
- **THEN** `cron.yaml` is updated with the new values and the scheduler hot-reloads

#### Scenario: Invalid schedule shows error
- **WHEN** the user enters an invalid cron expression and confirms
- **THEN** an inline error message is shown and the entry is not saved

#### Scenario: Escape cancels edit
- **WHEN** the user presses `esc` in the edit overlay
- **THEN** the overlay closes and no changes are written

### Requirement: Delete action removes a job with confirmation
Pressing `d` on a selected job SHALL show a confirmation prompt. Confirming SHALL remove the entry from `cron.yaml` and reload the scheduler. Cancelling SHALL leave the entry intact.

#### Scenario: Delete confirmation prompt shown
- **WHEN** the user presses `d` on a selected entry
- **THEN** a confirmation dialog appears asking the user to confirm deletion

#### Scenario: Confirmed delete removes entry
- **WHEN** the user confirms the delete prompt
- **THEN** the entry is removed from `cron.yaml` and the scheduler reloads

#### Scenario: Cancelled delete is a no-op
- **WHEN** the user presses `n` or `esc` at the delete prompt
- **THEN** the entry remains in `cron.yaml`

### Requirement: Run-now action fires a job immediately
Pressing `enter` or `r` on a selected job SHALL execute that job immediately (outside its schedule) without modifying `cron.yaml`.

#### Scenario: Run-now executes the entry
- **WHEN** the user presses `enter` on a selected entry
- **THEN** the entry's command is executed immediately as a subprocess

#### Scenario: Run-now output appears in log pane
- **WHEN** a run-now execution produces output
- **THEN** the log lines appear in the bottom pane prefixed with the job name

### Requirement: TUI is theme-aware using active ABBS bundle
The TUI SHALL load the active ABBS theme bundle at startup and apply its Dracula palette colors to all UI elements (borders, selection highlight, status indicators). When no bundle is configured, it SHALL fall back to the default Dracula palette.

#### Scenario: TUI loads active bundle colors
- **WHEN** the TUI starts and a bundle is active
- **THEN** border and selection colors match the active bundle's palette

#### Scenario: Fallback to default Dracula palette
- **WHEN** no bundle is configured
- **THEN** the TUI renders with the default Dracula colors (purple, green, cyan, red accents)

### Requirement: TUI layout splits vertically with job list on top and logs on bottom
The TUI SHALL use a vertical split: top 60% for the job list, bottom 40% for the log pane. The split ratio SHALL adjust gracefully on small terminals (minimum 6 rows per pane). The active pane SHALL be indicated by a highlighted border.

#### Scenario: Default split is 60/40
- **WHEN** the TUI renders on a normal-sized terminal
- **THEN** the job list occupies approximately 60% of the height and the log pane 40%

#### Scenario: Small terminal collapses log pane
- **WHEN** the terminal height is too small to show both panes at 6 rows minimum
- **THEN** the log pane is collapsed and only the job list is shown

#### Scenario: Tab switches active pane
- **WHEN** the user presses `tab`
- **THEN** focus toggles between the job list pane and the log pane
