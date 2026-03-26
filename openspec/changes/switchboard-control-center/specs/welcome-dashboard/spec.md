## REMOVED Requirements

### Requirement: Dashboard is persistent and does not auto-exit
**Reason**: Replaced by switchboard-tui. The switchboard has its own quit semantics (`q` / `ctrl+c` with optional confirmation when a job is running).
**Migration**: `orcai welcome` and `orcai-welcome` now launch the switchboard, which is equally persistent.

### Requirement: Dashboard displays ANSI/BBS banner header
**Reason**: Replaced by switchboard-tui. The ANSI/BBS banner is retained inside the switchboard layout.
**Migration**: No user-visible change — the banner still appears in the switchboard.

### Requirement: Dashboard shows one session card per active tmux window
**Reason**: Replaced by switchboard-tui Activity Feed. Session telemetry is now shown as feed entries in the switchboard's center column.
**Migration**: Session/window telemetry cards are replaced by Activity Feed entries. Telemetry data still arrives via busd.

### Requirement: Status indicator reflects streaming vs idle state
**Reason**: Replaced by switchboard-tui Activity Feed status badges (`▶ running`, `✓ done`, `✗ failed`).
**Migration**: Status is visible in the Activity Feed badge for each run entry.

### Requirement: Dashboard shows aggregate totals row
**Reason**: Removed in favor of per-run entries in the Activity Feed. Aggregate cost tracking is out of scope for v1 switchboard.
**Migration**: None — per-run output is available in the Activity Feed.

### Requirement: Dashboard subscribes to orcai.telemetry bus
**Reason**: Merged into switchboard-tui. The switchboard retains this bus subscription.
**Migration**: No change to busd behavior.

### Requirement: Window list refreshes periodically
**Reason**: Replaced by switchboard-tui. The switchboard manages its own refresh cycle.
**Migration**: No user-visible change.

### Requirement: Enter opens the provider picker popup
**Reason**: Replaced by the Agent Runner inline form in the switchboard left column. No tmux popup needed.
**Migration**: Use the switchboard Agent Runner section instead.

### Requirement: Footer shows chord-key hints
**Reason**: Replaced by the switchboard bottom keybinding bar.
**Migration**: Keybinding hints are now in the switchboard bottom bar.
