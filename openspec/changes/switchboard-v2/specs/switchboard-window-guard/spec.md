## ADDED Requirements

### Requirement: Window 0 is the permanent Switchboard and cannot be killed
The ORCAI tmux session SHALL designate window 0 as the permanent Switchboard window. The `^spc X` (kill-window) and `^spc x` (kill-pane) chord bindings SHALL refuse to operate on window 0. A tmux `before-kill-window` hook SHALL cancel any kill targeting window 0 in the `orcai` session.

#### Scenario: kill-window blocked on window 0
- **WHEN** the user invokes `^spc X` while window 0 is active
- **THEN** the window is not destroyed and a brief status-bar message informs the user

#### Scenario: kill-pane blocked when pane is the last in window 0
- **WHEN** the user invokes `^spc x` on the sole pane of window 0
- **THEN** the pane is not destroyed

#### Scenario: kill-window allowed on non-zero windows
- **WHEN** the user invokes `^spc X` while a window other than window 0 is active
- **THEN** the window is destroyed normally

### Requirement: ^spc t focuses window 0 directly
The `^spc t` chord binding SHALL navigate the tmux client to window 0 of the `orcai` session using `select-window -t orcai:0`. It SHALL NOT open a display-popup.

#### Scenario: ^spc t from any window returns to Switchboard
- **WHEN** the user presses `^spc t` from any window
- **THEN** the tmux client shows window 0 and the Switchboard TUI is visible

#### Scenario: ^spc t from window 0 is a no-op
- **WHEN** the user presses `^spc t` while already on window 0
- **THEN** no visible change occurs and no error is produced

### Requirement: ^spc n chord is removed
The `^spc n` chord binding SHALL be removed from the orcai-chord key table. The provider-picker entry point it exposed SHALL NOT be accessible via chord key.

#### Scenario: ^spc n produces no action
- **WHEN** the user presses `^spc n` inside an orcai session
- **THEN** no popup opens and no new window is created

### Requirement: New shell windows remain creatable via ^spc c
The `^spc c` chord binding SHALL continue to open a new plain tmux window (raw shell). This is the only supported way to create additional windows outside the Switchboard.

#### Scenario: ^spc c opens a new shell window
- **WHEN** the user presses `^spc c`
- **THEN** a new tmux window with a plain shell is created and focused
