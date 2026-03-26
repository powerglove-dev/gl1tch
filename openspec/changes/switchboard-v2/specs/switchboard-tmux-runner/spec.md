## ADDED Requirements

### Requirement: Pipelines and agents run in background tmux windows
When the user launches a pipeline or agent from the Switchboard, the runner SHALL create a new background tmux window named `orcai-<feedID>` in the current session. The pipeline or agent SHALL execute inside that window (via a shell command that streams output to a log file). The Switchboard feed entry for the job SHALL record the fully-qualified tmux window target.

#### Scenario: Pipeline launch creates a background tmux window
- **WHEN** the user selects a pipeline in the launcher and presses Enter
- **THEN** a new tmux window named `orcai-<feedID>` is created in the session
- **AND** the pipeline begins executing inside that window

#### Scenario: Agent launch creates a background tmux window
- **WHEN** the user submits a Quick Run agent prompt
- **THEN** a new tmux window named `orcai-<feedID>` is created in the session
- **AND** the agent begins executing inside that window

### Requirement: Multiple jobs can run concurrently
The Switchboard SHALL support multiple simultaneously active jobs. Each job SHALL have its own feed entry, background tmux window, and log file. There is no single-active-job restriction.

#### Scenario: Two pipelines run in parallel
- **WHEN** the user launches pipeline A and then pipeline B before A finishes
- **THEN** both jobs appear in the feed with status `running`
- **AND** each occupies its own tmux window

#### Scenario: Job cap enforced
- **WHEN** the number of active jobs reaches the maximum (default 8)
- **THEN** launching an additional job is blocked and a status message is displayed

### Requirement: Preview popup shows live log tail over the Switchboard
When the user selects an active or completed feed entry and presses Space (or Enter on the feed), the Switchboard SHALL display a preview popup overlaid on the current view. The popup SHALL show the last 30 lines of the job's log file, rendered inside a lipgloss border box centred at ~80% width and ~60% height. The popup SHALL update live while the job is running.

#### Scenario: Space opens preview popup
- **WHEN** a feed entry is highlighted and the user presses Space
- **THEN** a popup overlay appears showing the tail of the job log

#### Scenario: Preview popup updates live
- **WHEN** the job is still running and new output is written to the log
- **THEN** the popup refreshes to show the latest lines without the user taking any action

#### Scenario: Esc closes preview popup
- **WHEN** the preview popup is open and the user presses Esc
- **THEN** the popup closes and the Switchboard returns to normal view

### Requirement: Enter from preview popup navigates into the background tmux window
While the preview popup is open, pressing Enter SHALL close the popup and navigate the tmux client to the job's background window via `select-window`.

#### Scenario: Enter navigates to job window from preview
- **WHEN** the preview popup is open and the user presses Enter
- **THEN** the popup closes
- **AND** the tmux client switches to the job's background window
- **AND** the user can interact directly with the running or completed process

### Requirement: Background job windows are not listed in the status bar
Background job windows (windows created by the Switchboard for pipeline/agent runs) SHALL NOT appear as entries in the tmux status bar window list. The status bar window list SHALL remain empty (only showing the ORCAI label, hints, and clock).

#### Scenario: New job window does not appear in status bar
- **WHEN** a pipeline launches and its background tmux window is created
- **THEN** the status bar does not show a new window entry
