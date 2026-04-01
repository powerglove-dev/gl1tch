## Why

The switchboard has no dedicated surface for cron-scheduled jobs — users must navigate blindly to the `orcai-cron` tmux session to inspect schedules, and there is no interactive TUI for managing (editing, deleting, triggering) entries. Bringing cron into the switchboard panel model and giving `orcai-cron` a full ABBS-styled TUI closes this gap.

## What Changes

- Add a **Cron panel** to the switchboard alongside Pipelines, Agent Runner, and Inbox; keybind `c` focuses it, shows the next few upcoming scheduled jobs.
- Add `m` (manage) shortcut inside the Cron panel that switches focus to the `orcai-cron` tmux window.
- Build a **new `orcai-cron` BubbleTea TUI** (`cmd/orcai-cron/`) that runs inside the `orcai-cron` tmux session:
  - Top pane: scrollable/filterable job list (fuzzy search via `charmbracelet/bubbles/textinput`).
  - Bottom pane: live log viewer using `charmbracelet/log` output with job-name prefix on every line.
  - Actions: `e` edit, `d` delete, `enter` run-now.
- The TUI is theme-aware (reads the active ABBS bundle; uses Dracula palette styles).
- `orcai cron start` launches the TUI instead of the bare daemon; the scheduler continues to run inside the TUI's model.

## Capabilities

### New Capabilities
- `switchboard-cron-panel`: Cron widget embedded in the switchboard that displays upcoming scheduled jobs and provides a `c` focus shortcut and `m` manage shortcut.
- `orcai-cron-tui`: Full BubbleTea TUI for `orcai-cron` session — split-pane job list + log viewer with ABBS aesthetic, theme awareness, and CRUD actions on cron entries.

### Modified Capabilities
- `status-bar-session-controls`: The bottom-bar hint strip gains a `c` cron entry to match the other panel shortcuts shown there.

## Impact

- `internal/switchboard/switchboard.go` — new cron panel rendering + `c`/`m` key handlers.
- `cmd/orcai-cron/` — new binary entry point (BubbleTea TUI + embedded scheduler).
- `cmd/cmd_cron.go` — `cron start` updated to launch the TUI binary instead of the bare daemon command.
- `internal/cron/scheduler.go` — expose `NextRun(entry)` helper so the switchboard panel and TUI can show scheduled fire times.
- `internal/theme/` (or switchboard bundle loader) — TUI reads active bundle for Dracula palette colors.
- No breaking changes to `cron.yaml` schema or `internal/cron` package API; additive only.
