## MODIFIED Requirements (agent-context-panel)

### Requirement: Panel renders as a BBS sysop monitor
The sidebar panel view SHALL render as a BBS-style sysop node monitor with a full outer `╔═╗ ║ ╚═╝` box-drawing border spanning panel width, a `▒▒▒ ORCAI SYSOP MONITOR ▒▒▒` header block inside the top border, an active node count and current time on the header sub-line, and `╠══╣` horizontal dividers between each node section.

#### Scenario: Panel renders outer border
- **WHEN** the panel view is rendered
- **THEN** the first line is `╔` followed by `═` repeated to fill the width and `╗`, and the last line is `╚` … `╝`

#### Scenario: Panel renders sysop header
- **WHEN** the panel view is rendered
- **THEN** the header section contains `ORCAI SYSOP MONITOR` surrounded by `▒` block characters and a line showing `NODES:` count and current HH:MM time

### Requirement: Each session is rendered as a numbered node row
Each non-home tmux window with or without telemetry SHALL appear as a node section. The node section SHALL show: `NODE XX` label (1-based, zero-padded to 2 digits) followed by a status badge `[BUSY]`, `[IDLE]`, or `[WAIT]` on the first row; the provider name on the second row; input tokens, output tokens, and cost on the third row. Windows without telemetry SHALL show `[WAIT]` and `no data` in place of metrics.

#### Scenario: Node rows rendered for each window
- **WHEN** N non-home windows exist
- **THEN** N node sections appear, labelled `NODE 01` through `NODE N`

#### Scenario: BUSY status for streaming session
- **WHEN** a session has received a `status: "streaming"` telemetry event
- **THEN** its node row shows `[BUSY]` in green

#### Scenario: IDLE status for done session
- **WHEN** a session has received a `status: "done"` telemetry event
- **THEN** its node row shows `[IDLE]` in dim teal

#### Scenario: WAIT status for no-data session
- **WHEN** a window exists but no telemetry has been received
- **THEN** its node row shows `[WAIT]` in yellow and metrics display `no data`

### Requirement: Activity log section shows recent telemetry events
Below the node sections, a log section header `── ACTIVITY LOG ──` and up to 12 log entries SHALL be rendered. Each entry shows `HH:MM NODE XX <event>` with optional cost for done events. Entries are ordered newest-first. When no events have occurred, the log section shows `no activity`.

#### Scenario: Activity log populated on telemetry event
- **WHEN** a telemetry event is received
- **THEN** a new entry appears at the top of the activity log

#### Scenario: Activity log caps at 12 entries
- **WHEN** more than 12 telemetry events have been received
- **THEN** only the 12 most recent entries are shown

#### Scenario: Activity log shows no activity when empty
- **WHEN** no telemetry events have been received
- **THEN** the log section displays `no activity` in dim text

### Requirement: Panel toggles on the current active window
The `ctrl+space t` chord SHALL show or hide the panel pane on whichever tmux window the user is currently in. The panel SHALL NOT be restricted to window 0. Each window tracks its own panel visibility independently.

#### Scenario: Panel opens on current window
- **WHEN** the user presses `ctrl+space t` in window 2
- **THEN** the panel appears as a left-side split on window 2, not on window 0

#### Scenario: Panel closes on current window
- **WHEN** the panel is visible on window 2 and the user presses `ctrl+space t`
- **THEN** the panel pane on window 2 is killed

#### Scenario: Panels on different windows are independent
- **WHEN** the panel is open on window 1 and the user navigates to window 2 and presses `ctrl+space t`
- **THEN** a new panel opens on window 2 while window 1's panel remains

### Requirement: Panel width is 30% of the current window
The panel pane SHALL be spawned at 30% of the current window width (`-l 30%`). The panel SHALL resize to match 30% on `tea.WindowSizeMsg`.

#### Scenario: Panel spawns at 30% width
- **WHEN** `RunToggle()` spawns a new panel
- **THEN** the tmux split-window command uses `-l 30%`

### Requirement: Cursor-selected node is highlighted
The node section at the cursor position SHALL be highlighted with a selection background (`\x1b[48;5;236m`) on the `NODE XX [STATUS]` line.

#### Scenario: Selected node has accent background
- **WHEN** cursor is at node index I
- **THEN** node I's header line renders with the selection background colour

### Requirement: Footer shows navigation hints inside the border
The footer line (last inner line before `╚═╝`) SHALL show `enter focus · x kill · ↑↓ nav` in dim blue.
