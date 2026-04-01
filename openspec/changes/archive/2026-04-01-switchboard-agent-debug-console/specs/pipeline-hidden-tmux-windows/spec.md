## ADDED Requirements

### Requirement: Agent jobs create a dedicated tmux window
When the Switchboard launches an agent job via `handleEnter()`, it SHALL create a new tmux window in the current tmux session named `orcai-<feedID>` using `tmux new-window -d -n orcai-<feedID>`. The window SHALL start in the current working directory.

#### Scenario: Window created on job launch
- **WHEN** the user submits an agent prompt
- **THEN** a new tmux window named `orcai-<feedID>` is created in the session
- **THEN** the window is created in detached mode (`-d`) so the current window remains visible

### Requirement: Agent job windows are hidden from the status bar
Immediately after creating the window, the Switchboard SHALL run `tmux set-window-option -t <session>:orcai-<feedID> hide-from-statusbar on`. Only the Switchboard window SHALL be visible in the tmux status bar.

#### Scenario: Window hidden from status bar
- **WHEN** a job window is created
- **THEN** `hide-from-statusbar` is set to `on` for that window
- **THEN** the tmux status bar does not show the new window tab

#### Scenario: Graceful failure when hide-from-statusbar unsupported
- **WHEN** the tmux version does not support `hide-from-statusbar`
- **THEN** the window is still created and the job proceeds normally
- **THEN** an error is NOT returned; the failure is silently ignored

### Requirement: Job output is echoed into the tmux window
As each output line arrives from the agent process, the Switchboard SHALL send the line to the tmux window via `tmux send-keys -t orcai-<feedID> "<line>" Enter`. This gives the user a shell-history-like record in the window.

#### Scenario: Output line echoed to window
- **WHEN** a `FeedLineMsg` is received for a running job
- **THEN** the line is sent to the job's tmux window via `tmux send-keys`

### Requirement: Job windows persist after completion
When a job finishes (success or failure), the Switchboard SHALL NOT close the associated tmux window. The window SHALL remain open for the user to inspect.

#### Scenario: Window survives job completion
- **WHEN** a job transitions to `FeedDone` or `FeedFailed`
- **THEN** the tmux window still exists
- **THEN** `tmux list-windows` includes `orcai-<feedID>`

### Requirement: jobHandle carries the tmux window name
The `jobHandle` struct SHALL include a `tmuxWindow string` field set to the window name at job creation. This field is used by the debug popup to call `tmux capture-pane`.

#### Scenario: jobHandle has tmuxWindow
- **WHEN** a job is launched
- **THEN** `activeJob.tmuxWindow` equals `"orcai-<feedID>"`
