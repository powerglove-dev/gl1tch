---
title: "Cron Scheduling"
description: "Schedule recurring pipelines and agents with human-readable next-run times."
order: 9
---

gl1tch's cron system executes pipelines and agents on a schedule, specified as standard 5-field cron expressions. Entries are stored in `~/.config/glitch/cron.yaml`, watched for changes, and hot-reloaded by the daemon. The TUI shows upcoming jobs with their next fire time in human-readable relative format (e.g. "in 4m", "in 2h 30m").


## Architecture

The cron system has three layers:

1. **Scheduler** (`internal/cron/scheduler.go`) — wraps `robfig/cron` and adds hot-reload via `fsnotify`. When a pipeline or agent runs, it publishes events to the internal event bus for logging and UI updates.
2. **Config** (`internal/cron/config.go`) — reads/writes `~/.config/glitch/cron.yaml` atomically. Entries can be upserted from the agent runner modal or pipeline launcher without restarting the daemon.
3. **Helpers** (`internal/cron/helpers.go`) — `NextRun()` parses a schedule and returns the next fire time; `FormatRelative()` converts that to human-readable strings like "in 2d 3h".

Entry execution spawns a subprocess with `os/exec`. Output is logged to `~/.local/share/glitch/cron.log` and prefixed with the entry name for correlation.


## Technologies

- **robfig/cron/v3** — standard 5-field cron parser and scheduler loop. Supports minute, hour, day-of-month, month, and day-of-week fields. No timezone logic; uses the system clock.
- **fsnotify** — watches `~/.config/glitch/cron.yaml` for changes and triggers a hot-reload within ~1 second.
- **charmbracelet/log** — structured logging with timestamps, output to both stderr and the cron log file.


## Concepts

**Entry** — a single scheduled job. Has a name, cron expression, kind (pipeline or agent), and target file/name.

**Schedule** — a 5-field cron expression: minute, hour, day-of-month, month, day-of-week. Supports `*`, ranges (`1-5`), steps (`*/15`), and lists (`1,3,5`). No seconds or timezone. Evaluated in the system timezone.

**Kind** — either `pipeline` or `agent`. Determines how Target is resolved: as a `.pipeline.yaml` path (pipeline) or an agent name (agent).

**Next-run time** — calculated by `NextRun()` after parsing the schedule. Displayed as a human-readable relative duration: "now", "in 4m", "in 2h 30m", "in 3d", etc.

**Hot-reload** — the scheduler watches `~/.config/glitch/cron.yaml` for writes. When the file changes, all entries are re-parsed and the scheduler is updated in-place (no restart required).


## Configuration

Cron entries are stored in `~/.config/glitch/cron.yaml`. Each entry is a YAML object with these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Human-readable label for this job (must be unique) |
| `schedule` | string | yes | 5-field cron expression (minute hour dom month dow) |
| `kind` | string | yes | Either `pipeline` or `agent` |
| `target` | string | yes | For pipeline: path to `.pipeline.yaml`. For agent: agent name. |
| `args` | map[string]any | no | Key-value arguments passed to the target |
| `input` | string | no | Optional input string mapped to `{{param.input}}` in pipelines |
| `timeout` | string | no | Maximum duration (e.g. `"5m"`, `"30s"`). Zero = no timeout. |
| `working_dir` | string | no | Working directory for the subprocess. Empty inherits the daemon's cwd. |

The file must exist at `~/.config/glitch/cron.yaml`. If not present, `glitch cron start` creates it on first run.


## Examples

**Basic pipeline schedule** — run a cleanup pipeline daily at 2 AM:

```yaml
entries:
  - name: nightly-cleanup
    schedule: "0 2 * * *"
    kind: pipeline
    target: ./pipelines/cleanup.pipeline.yaml
    timeout: "10m"
    working_dir: /home/user/projects/my-repo
```

**Agent with input** — run an agent every 15 minutes, passing custom input:

```yaml
entries:
  - name: poll-status
    schedule: "*/15 * * * *"
    kind: agent
    target: status-checker
    input: "slack-alerts"
    args:
      channel: "#ops"
      verbose: true
```

**Weekdays only** — run a report at 9 AM on weekdays (Monday–Friday):

```yaml
entries:
  - name: weekday-report
    schedule: "0 9 * * 1-5"
    kind: pipeline
    target: ./pipelines/report.pipeline.yaml
    timeout: "30m"
```

**Run immediately** (for testing):

```
glitch cron list  # shows all entries with next-run times
```

Then from the TUI (`glitch cron start` or press `m` from the switchboard Cron panel) to execute immediately outside its schedule.


## Commands

### `glitch cron start`

Start the cron daemon in a detached tmux session called `glitch-cron`. The session runs the BubbleTea TUI by default (or `glitch cron run` in non-interactive contexts like CI). Loads `~/.config/glitch/cron.yaml` and begins watching it for changes.

**Flags:**
- `--force` — kill an existing `glitch-cron` session before starting.

### `glitch cron stop`

Kill the `glitch-cron` tmux session and stop the daemon.

### `glitch cron list`

Print all configured entries in a table, showing name, schedule, kind, target, and next fire time in human-readable format. Next-run time includes both a relative duration ("in 4m") and an absolute timestamp ("14:30 UTC").

### `glitch cron logs`

Tail the cron daemon log file at `~/.local/share/glitch/cron.log`. Each log line is prefixed with the entry name for correlation.

### `glitch cron run` (internal)

The daemon entry point; invoked by `glitch cron start` inside the tmux session. Not intended for direct user invocation.

### `glitch cron tui` (internal)

Launches the BubbleTea TUI for job management. Embedded inside the daemon; hot-reloads on config changes. Provides a filterable job list, run-now action, and live log tail.


## Next-run time format

Times are displayed as human-readable relative durations:

- **"now"** — within 1 second
- **"in Xm"** — e.g. "in 4m" (less than 1 hour)
- **"in Xh"** — e.g. "in 2h" (less than 1 day)
- **"in Xh Ym"** — e.g. "in 2h 30m" (less than 1 day, both hours and minutes)
- **"in Xd"** — e.g. "in 3d" (1+ days, same hour tomorrow or later)
- **"in Xd Yh"** — e.g. "in 3d 2h" (1+ days with hours)

This format is used in `glitch cron list` output, the switchboard Cron panel, and the glitch-cron TUI.


## See Also

- [Pipelines](./pipelines.md) — running inline or scheduled pipelines
- [Agents](./agents.md) — scheduling agent runs with recurring parameters
- [Switchboard](./switchboard.md) — cron panel in the main dashboard (press `c` to focus, `m` to open the TUI)

