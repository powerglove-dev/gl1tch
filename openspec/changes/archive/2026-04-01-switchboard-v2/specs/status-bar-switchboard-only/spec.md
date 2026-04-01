## ADDED Requirements

### Requirement: Status bar shows only ORCAI label, key hints, and clock
The tmux status bar SHALL show `ORCAI 0:SWITCHBOARD` on the left, key hints `^spc t switchboard  ^spc c new-shell` on the right, and the current time (`%H:%M`) on the right. No tmux window list SHALL appear in the centre.

#### Scenario: Status bar left shows ORCAI label
- **WHEN** an orcai session is running
- **THEN** the status bar left section contains `ORCAI 0:SWITCHBOARD`

#### Scenario: Status bar right shows key hints and clock
- **WHEN** an orcai session is running
- **THEN** the status bar right section contains `^spc t switchboard` and `^spc c new-shell` and the current time

#### Scenario: No window entries in status bar centre
- **WHEN** the orcai session has multiple windows open
- **THEN** the status bar centre is empty — no window names or indices are displayed

### Requirement: window-status-format is blank
The tmux `window-status-format` and `window-status-current-format` options SHALL be set to empty strings so that no window appears in the status bar list regardless of how many windows exist.

#### Scenario: window-status-format empty
- **WHEN** the orcai session config is applied
- **THEN** `tmux show-options -g window-status-format` returns an empty string

#### Scenario: window-status-current-format empty
- **WHEN** the orcai session config is applied
- **THEN** `tmux show-options -g window-status-current-format` returns an empty string
