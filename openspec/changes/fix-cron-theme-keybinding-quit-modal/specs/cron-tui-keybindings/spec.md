## ADDED Requirements

### Requirement: Theme switcher opens via chord from cron session
Pressing `^spc m` from the `orcai-cron` tmux session SHALL open the theme picker in the switchboard, identical to pressing `^spc m` from any other ORCAI context. The cron TUI SHALL continue to receive theme change events passively via busd subscription.

#### Scenario: Theme picker opens from cron session
- **WHEN** the user presses `^spc m` while the active tmux session is `orcai-cron`
- **THEN** the tmux focus switches to the `orcai` session and the theme picker overlay opens in the switchboard

#### Scenario: Theme change applies to cron TUI
- **WHEN** the user selects a new theme in the switchboard theme picker
- **THEN** the cron TUI SHALL update its colors via the busd `theme.changed` event (existing behavior, unchanged)

### Requirement: Quit shortcut removed from cron TUI
The cron TUI SHALL NOT respond to `q`, `ctrl+c`, or `esc` as quit triggers. The cron TUI lifecycle is fully managed by ORCAI and tmux; users MUST use `^spc q` to quit ORCAI.

#### Scenario: q key does not quit cron TUI
- **WHEN** the user presses `q` in the cron TUI (not while filtering)
- **THEN** nothing happens; the cron TUI remains open

#### Scenario: ctrl+c does not quit cron TUI
- **WHEN** the user presses `ctrl+c` in the cron TUI
- **THEN** nothing happens; the cron TUI remains open

#### Scenario: esc on jobs pane does not quit cron TUI
- **WHEN** the user presses `esc` while on the jobs pane (not while filtering)
- **THEN** nothing happens; the cron TUI remains open

### Requirement: Quit confirmation modal removed from cron TUI
The cron TUI SHALL NOT display a quit confirmation dialog. All quit-confirm state (`quitConfirm`), rendering (`viewQuitConfirm`), and key handling (`handleQuitConfirmKey`) SHALL be removed.

#### Scenario: No quit modal appears
- **WHEN** any previously quit-triggering key is pressed
- **THEN** no quit confirmation modal is rendered

### Requirement: Chord quit routes to switchboard from cron session
Pressing `^spc q` from the `orcai-cron` tmux session SHALL route to the switchboard quit flow, identical to pressing `^spc q` from any other ORCAI session.

#### Scenario: Chord quit from cron session
- **WHEN** the user presses `^spc q` while the active tmux session is `orcai-cron`
- **THEN** the tmux focus switches to the `orcai` session and the switchboard quit flow is triggered
