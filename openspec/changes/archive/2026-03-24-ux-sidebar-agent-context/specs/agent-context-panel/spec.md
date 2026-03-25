## ADDED Requirements

### Requirement: Leader key is ctrl+space
The tmux leader key SHALL be `ctrl+space` (`C-Space`). The previous backtick (`` ` ``) binding SHALL be removed. All existing chord actions (quit, detach, reload, new, ollama, opsx, toggle sidebar) SHALL remain reachable via `ctrl+space` followed by their chord key.

#### Scenario: ctrl+space enters chord table
- **WHEN** the user presses `ctrl+space` in any tmux pane
- **THEN** tmux switches to the `orcai-chord` key table

#### Scenario: Backtick no longer activates chord table
- **WHEN** the user presses `` ` `` in any tmux pane
- **THEN** tmux does NOT switch to the `orcai-chord` key table and the character passes through to the pane

### Requirement: ESC is not intercepted globally
The global `bind-key -n Escape` binding SHALL be removed. ESC SHALL pass through to the active pane unmodified, allowing CLI tools (e.g. agent cancellation) to receive it.

#### Scenario: ESC reaches active pane
- **WHEN** the user presses `ESC` while a CLI tool is running in the active pane
- **THEN** the ESC key is delivered to the CLI tool, not intercepted by tmux

#### Scenario: No focus-steal on ESC
- **WHEN** the user presses `ESC` in any pane
- **THEN** the active pane does not change to pane `.0`

### Requirement: Sidebar is hidden by default and togglable
The sidebar SHALL be hidden when an orcai session starts. The user SHALL be able to show or hide the sidebar by pressing `ctrl+; t`. The sidebar toggle state SHALL persist across keystrokes within a session (show stays shown until toggled off).

#### Scenario: Session starts with sidebar hidden
- **WHEN** orcai starts a new tmux session
- **THEN** no sidebar pane is created in window 0

#### Scenario: ctrl+space t shows hidden sidebar
- **WHEN** the sidebar is hidden and the user presses `ctrl+space t`
- **THEN** the sidebar pane is created at 25% width on the left of window 0

#### Scenario: ctrl+space t hides visible sidebar
- **WHEN** the sidebar is visible and the user presses `ctrl+space t`
- **THEN** the sidebar pane is killed and the window returns to full width

### Requirement: Agent context panel displays per-session telemetry
The sidebar SHALL display a list of active tmux windows. For each window that has published telemetry, the sidebar SHALL show: provider name, current status (running / idle), total input tokens, total output tokens, and estimated cost in USD. Windows with no telemetry data SHALL show "no data" in place of metrics.

#### Scenario: Session with telemetry shows metrics
- **WHEN** a session has published at least one `orcai.telemetry` bus event
- **THEN** the sidebar shows that session's provider, status, token counts, and cost

#### Scenario: Session without telemetry shows placeholder
- **WHEN** a session has no telemetry data
- **THEN** the sidebar displays "no data" for that session's metrics

#### Scenario: Running status shown during streaming
- **WHEN** a `orcai.telemetry` event with `status: streaming` is received for a session
- **THEN** the sidebar shows "● running" for that session

#### Scenario: Idle status shown after stream completes
- **WHEN** a `orcai.telemetry` event with `status: done` is received for a session
- **THEN** the sidebar shows "○ idle" for that session

### Requirement: Bus address persisted for sidebar connection
Bootstrap SHALL write the event bus address to `~/.config/orcai/bus.addr` after the bus starts. The sidebar SHALL read this file on startup and subscribe to the `orcai.telemetry` topic. If the file is not present at startup the sidebar SHALL retry for up to 3 seconds before proceeding without telemetry.

#### Scenario: Bus address file written on start
- **WHEN** orcai bootstrap starts the event bus
- **THEN** `~/.config/orcai/bus.addr` contains the bus TCP address

#### Scenario: Sidebar connects to bus
- **WHEN** the sidebar starts and `bus.addr` exists
- **THEN** the sidebar subscribes to `orcai.telemetry` and receives subsequent events

#### Scenario: Sidebar starts without bus addr
- **WHEN** the sidebar starts and `bus.addr` does not exist after 3 seconds
- **THEN** the sidebar renders without telemetry data (no crash)

### Requirement: Chatui publishes telemetry events
Chatui SHALL publish a `orcai.telemetry` bus event when a stream starts and again when `StreamDone` is received. The event payload SHALL be JSON containing: `session_id`, `provider`, `status` (`"streaming"` or `"done"`), `input_tokens`, `output_tokens`, `cost_usd`.

#### Scenario: Streaming event published on stream start
- **WHEN** chatui begins streaming a response
- **THEN** a `orcai.telemetry` event with `status: "streaming"` is published to the bus

#### Scenario: Done event published on StreamDone
- **WHEN** chatui receives a `StreamDone` message
- **THEN** a `orcai.telemetry` event with `status: "done"` and token counts and cost is published to the bus
