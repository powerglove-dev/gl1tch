## REMOVED Requirements

### Requirement: Status bar shows session control hints
**Reason**: Replaced by `status-bar-switchboard-only` which defines a cleaner status bar with no window list and updated hints (`^spc t switchboard` and `^spc c new-shell` only). The `^spc n new` and `^spc p build` hints are removed because `^spc n` is being deleted and `^spc p` is not being promoted.
**Migration**: The new status-right format is `^spc t switchboard  ^spc c new-shell   %H:%M`. Remove the old `^spc n new  ^spc c win  ^spc x kill  ^spc t switchboard` format from `internal/bootstrap/bootstrap.go`.

### Requirement: New-session and prompt-builder removed from sidebar footer
**Reason**: Sidebar/switchboard footer now shows only navigation hints. This requirement is satisfied by the existing implementation; no further change needed. Removing from active spec tracking to reduce noise.
**Migration**: No action required — footer already omits these hints.

## MODIFIED Requirements

### Requirement: Clock remains visible
The tmux status bar right section SHALL continue to display the current time in `%H:%M` format. The clock SHALL appear as the rightmost element of the status-right string, after the key hints.

#### Scenario: Clock remains visible
- **WHEN** an orcai session is running
- **THEN** the tmux status bar right side shows the current time in HH:MM format
