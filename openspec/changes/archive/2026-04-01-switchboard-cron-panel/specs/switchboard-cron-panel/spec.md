## ADDED Requirements

### Requirement: Cron panel is accessible via keyboard shortcut
The switchboard SHALL focus the Cron panel when the user presses `c` from any non-modal context.

#### Scenario: `c` key focuses Cron panel
- **WHEN** no modal or overlay is active and the user presses `c`
- **THEN** the Cron panel receives focus and the panel header is highlighted

#### Scenario: `c` key is a no-op inside modals
- **WHEN** an agent modal or pipeline overlay is active
- **THEN** pressing `c` does not change panel focus

### Requirement: Cron panel displays upcoming scheduled jobs
The Cron panel SHALL show the next scheduled fire time for each entry in `cron.yaml`, sorted ascending by next-run time. The panel SHALL display at minimum: job name, kind (pipeline/agent), schedule expression, and formatted next-run time (e.g. "in 5m" or "in 2h 30m").

#### Scenario: Panel renders job list from cron.yaml
- **WHEN** the Cron panel is visible and `cron.yaml` contains one or more entries
- **THEN** each entry appears as a row with name, kind, and next-run time

#### Scenario: Panel shows empty state
- **WHEN** the Cron panel is visible and `cron.yaml` has no entries
- **THEN** the panel displays a placeholder message (e.g. "no scheduled jobs")

#### Scenario: Next-run times are relative
- **WHEN** an entry's next scheduled time is in the future
- **THEN** the time is shown as a human-readable relative duration (e.g. "in 4m")

### Requirement: Manage shortcut switches to orcai-cron window
From within the Cron panel, pressing `m` SHALL switch the active tmux window to the `orcai-cron` session.

#### Scenario: `m` navigates to orcai-cron session
- **WHEN** the Cron panel is focused and the user presses `m`
- **THEN** the terminal switches to the `orcai-cron` tmux window

#### Scenario: `m` auto-starts daemon if not running
- **WHEN** the user presses `m` and the `orcai-cron` tmux session does not exist
- **THEN** the session is created (running the cron TUI) before switching to it

### Requirement: Cron panel renders with ABBS panel-header sprite
The Cron panel SHALL use `PanelHeader(bundle, "cron", w)` to render its header, consistent with Pipelines, Agent Runner, and Inbox panels.

#### Scenario: Cron panel header uses ANSI box style
- **WHEN** the Cron panel is rendered
- **THEN** it has the same ANSI box header as the other switchboard panels

### Requirement: Bottom-bar hint strip includes cron shortcut
The switchboard bottom-bar hint strip SHALL include a `c cron` entry alongside the other panel shortcuts.

#### Scenario: Hint strip shows c cron
- **WHEN** the switchboard is rendered
- **THEN** the bottom bar shows `c cron` as one of the navigation hints
