## ADDED Requirements

### Requirement: Enter on signal board row opens debug popup
When the signal board has focus and the user presses `enter` on a selected row, the Model SHALL set `debugPopupOpen = true` and record the associated `feedID` in `debugPopupJobID`.

#### Scenario: Enter opens popup
- **WHEN** signal board has focus and user presses `enter`
- **THEN** `debugPopupOpen` is set to true
- **THEN** `debugPopupJobID` is set to the selected row's job ID

### Requirement: Debug popup renders as 80%-wide overlay
When `debugPopupOpen` is true, `View()` SHALL render a bordered overlay box occupying 80% of the terminal width, centered horizontally, on top of the normal layout. The popup height SHALL be 80% of the terminal height.

#### Scenario: Popup visible when open
- **WHEN** `debugPopupOpen` is true
- **THEN** the rendered output contains the popup border characters
- **THEN** the popup does not replace the underlying layout but overlays it

### Requirement: Popup displays captured tmux pane content
The popup body SHALL contain the output of `tmux capture-pane -t <tmuxWindow> -p` for the job's associated tmux window. If the window no longer exists the popup SHALL show a "window closed" message.

#### Scenario: Pane content shown
- **WHEN** the popup is open and the job's tmux window exists
- **THEN** the captured pane text is displayed inside the popup

#### Scenario: Window closed message
- **WHEN** the popup is open and the job's tmux window no longer exists
- **THEN** the popup body shows "window closed or not available"

### Requirement: Esc closes the debug popup
When `debugPopupOpen` is true, pressing `esc` SHALL set `debugPopupOpen = false` and return to normal navigation.

#### Scenario: Esc dismisses popup
- **WHEN** the popup is open and user presses `esc`
- **THEN** `debugPopupOpen` is set to false
- **THEN** the normal layout is displayed without overlay

### Requirement: Second enter attaches to the tmux window
When the popup is open, pressing `enter` a second time SHALL run `tmux select-window -t <tmuxWindow>` via `os/exec`, surfacing the agent's window in the terminal multiplexer for interactive use.

#### Scenario: Enter in popup attaches window
- **WHEN** the popup is open and user presses `enter`
- **THEN** `tmux select-window` is called with the job's window name
- **THEN** the popup closes
