## ADDED Requirements

### Requirement: Dashboard is persistent and does not auto-exit
Window 0 SHALL display the live dashboard indefinitely after launch. The dashboard SHALL NOT exit to `$SHELL` on arbitrary keypresses. Only an explicit quit action (`q` or `ctrl+c`) SHALL trigger the shell handoff.

#### Scenario: Dashboard stays open after launch
- **WHEN** orcai starts and window 0 opens
- **THEN** the dashboard remains visible and does not exit until the user presses `q` or `ctrl+c`

#### Scenario: Arbitrary keypresses do not close dashboard
- **WHEN** the user presses any key other than `q`, `ctrl+c`, or `enter`
- **THEN** the dashboard remains open and does not exit to shell

#### Scenario: q quits to shell
- **WHEN** the user presses `q`
- **THEN** the dashboard exits and the process is replaced by `$SHELL`

### Requirement: Dashboard displays ANSI/BBS banner header
The existing `buildWelcomeArt` banner SHALL be displayed at the top of the dashboard. The banner SHALL use the Dracula palette (purple `#6272a4` / `#bd93f9`, pink `#ff79c6`, teal) and box-drawing characters (`╔ ═ ╗ ║ ╠ ╣ ╚ ╝`). The banner SHALL scale to terminal width.

#### Scenario: Banner renders at top
- **WHEN** the dashboard view is rendered
- **THEN** the first lines contain the ORCAI banner with box-drawing borders and the "ORCAI" logotype

#### Scenario: Banner scales to terminal width
- **WHEN** the terminal is resized
- **THEN** the banner redraws at the new width without truncation or overflow

### Requirement: Dashboard shows one session card per active tmux window
The dashboard SHALL render one card for each non-home tmux window in the `orcai` session. Each card SHALL display the window name, provider, status indicator (● streaming / ○ idle), input token count, output token count, and estimated cost in USD. Windows with no received telemetry SHALL display "no data" in the metrics area. Cards SHALL use ANSI box-drawing borders in the Dracula palette.

#### Scenario: Card rendered for each active window
- **WHEN** the orcai session has N non-home windows
- **THEN** the dashboard shows N session cards

#### Scenario: Card shows telemetry data
- **WHEN** a window has received at least one `orcai.telemetry` event
- **THEN** its card shows provider name, status icon, input tokens (rounded to k), output tokens, and cost formatted as `$0.000`

#### Scenario: Card shows no-data placeholder
- **WHEN** a window has not yet received any telemetry
- **THEN** its card shows "no data" in the metrics area

#### Scenario: No sessions shows empty state
- **WHEN** no non-home windows exist
- **THEN** the dashboard shows an "no active sessions" message instead of cards

### Requirement: Status indicator reflects streaming vs idle state
A card's status indicator SHALL update in real time as telemetry events arrive. A `status: "streaming"` event SHALL show `●` in green. A `status: "done"` event SHALL show `○` in the dim colour.

#### Scenario: Streaming status shown in green
- **WHEN** a `orcai.telemetry` event with `status: "streaming"` is received
- **THEN** the corresponding card displays `●` in green (`\x1b[38;5;84m`)

#### Scenario: Idle status shown dimmed
- **WHEN** a `orcai.telemetry` event with `status: "done"` is received
- **THEN** the corresponding card displays `○` in dim teal (`\x1b[38;5;66m`)

### Requirement: Dashboard shows aggregate totals row
Below all session cards the dashboard SHALL render a single totals row summing input tokens, output tokens, and cost across all sessions that have telemetry data. The row SHALL be clearly labelled "TOTAL".

#### Scenario: Totals row sums all sessions
- **WHEN** multiple sessions have telemetry data
- **THEN** the totals row shows the sum of all input tokens, output tokens, and cost

#### Scenario: Totals row shows zero when no data
- **WHEN** no sessions have telemetry data
- **THEN** the totals row shows 0 for all fields

### Requirement: Dashboard subscribes to orcai.telemetry bus
The dashboard SHALL connect to the event bus via `~/.config/orcai/bus.addr` on startup and subscribe to the `orcai.telemetry` topic. Each received event SHALL update the corresponding session card immediately. The dashboard SHALL retry reading `bus.addr` for up to 3 seconds before proceeding without telemetry.

#### Scenario: Bus connection established on startup
- **WHEN** the dashboard starts and `bus.addr` exists
- **THEN** the dashboard connects to the bus and begins receiving telemetry events

#### Scenario: Bus unavailable — dashboard still renders
- **WHEN** `bus.addr` does not exist after the 3-second retry window
- **THEN** the dashboard renders with "no data" for all sessions and does not crash

#### Scenario: Telemetry event updates card in real time
- **WHEN** an `orcai.telemetry` event is received for a window
- **THEN** that window's card updates without requiring a manual refresh

### Requirement: Window list refreshes periodically
The dashboard SHALL refresh the list of active tmux windows every 5 seconds via a tick command. New windows SHALL appear as cards; killed windows SHALL be removed.

#### Scenario: New window appears after tick
- **WHEN** a new tmux window is created and 5 seconds elapse
- **THEN** a new card appears in the dashboard for that window

#### Scenario: Killed window removed after tick
- **WHEN** a tmux window is killed and 5 seconds elapse
- **THEN** its card is removed from the dashboard

### Requirement: Enter opens the provider picker popup
Pressing `Enter` on the dashboard SHALL open the provider/model picker in a tmux display-popup, identical to the current welcome screen behaviour.

#### Scenario: Enter opens picker
- **WHEN** the user presses `Enter`
- **THEN** a tmux display-popup opens running `orcai _picker`

### Requirement: Footer shows chord-key hints
The dashboard footer SHALL display `^spc n new · ^spc p build` hints in dim blue, consistent with the status-bar hints.

#### Scenario: Footer shows navigation hints
- **WHEN** the dashboard is rendered
- **THEN** the footer contains `^spc n new` and `^spc p build`
