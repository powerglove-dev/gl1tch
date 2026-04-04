---
title: "Cron Scheduling"
description: "Schedule your pipelines to run automatically — wake up to results already waiting in your workspace."
order: 9
---

Set a pipeline on a schedule and stop thinking about it. Your morning git digest runs at 8 AM. Your nightly docs audit finishes while you sleep. Your weekly dependency check fires every Monday. gl1tch runs your pipelines on a cron schedule and logs the results — you check in when you're ready.


## Quick Start

Create `~/.config/glitch/cron.yaml` with your first scheduled pipeline:

```yaml
entries:
  - name: git-digest
    schedule: "0 8 * * 1-5"
    kind: pipeline
    target: git-digest
    timeout: "5m"
    working_dir: ~/projects/my-repo
```

Start the scheduler:

```bash
glitch cron start
```

Check that it registered:

```bash
glitch cron list
```

Your pipeline now runs every weekday at 8 AM without you touching it. When it fires, gl1tch picks it up:

![gl1tch launching a scheduled pipeline](/screenshots/cron/cron-pipeline-launching.png)


## How Schedules Work

Schedules use standard 5-field cron expressions:

```text
┌───── minute (0–59)
│ ┌───── hour (0–23)
│ │ ┌───── day of month (1–31)
│ │ │ ┌───── month (1–12)
│ │ │ │ ┌───── day of week (0–6, Sun=0)
│ │ │ │ │
* * * * *
```

Common patterns:

| Schedule | Meaning |
|----------|---------|
| `0 8 * * 1-5` | 8 AM, Monday through Friday |
| `0 2 * * *` | 2 AM every day |
| `*/15 * * * *` | Every 15 minutes |
| `0 9 * * 1` | 9 AM every Monday |
| `0 * * * *` | Every hour on the hour |
| `30 * * * *` | Every hour at :30 |

Schedules run in your system timezone. No seconds field. No timezone suffix.


## Commands

### `glitch cron start`

Start the scheduler in your workspace. Loads `~/.config/glitch/cron.yaml` and watches it for changes — edit the file and your schedule updates within a second, no restart required.

```bash
glitch cron start
glitch cron start --force   # restart if already running
```

### `glitch cron stop`

Stop the scheduler.

```bash
glitch cron stop
```

### `glitch cron list`

Show all scheduled entries with their next fire time in plain English.

```bash
glitch cron list
```

### `glitch cron logs`

Tail the scheduler log. Each line is prefixed with the entry name so you can follow multiple jobs at once.

```bash
glitch cron logs
```


## Configuration Reference

All scheduled jobs live in `~/.config/glitch/cron.yaml`. The file is hot-reloaded on save.

```yaml
entries:
  - name: <string>          # unique name for this job
    schedule: <string>      # 5-field cron expression
    kind: pipeline          # "pipeline" or "agent"
    target: <string>        # pipeline name or path to .pipeline.yaml
    timeout: <string>       # optional: "5m", "30s" — zero means no timeout
    working_dir: <string>   # optional: working directory for the job
    input: <string>         # optional: maps to {{param.input}} in pipelines
    args:                   # optional: key-value args passed to the target
      key: value
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Unique label shown in `glitch cron list` |
| `schedule` | yes | 5-field cron expression |
| `kind` | yes | `pipeline` or `agent` |
| `target` | yes | Pipeline name or path to a `.pipeline.yaml` |
| `timeout` | no | Max run duration — e.g. `"10m"`, `"30s"` |
| `working_dir` | no | Directory the job runs in; inherits your workspace if empty |
| `input` | no | String mapped to `{{param.input}}` inside the pipeline |
| `args` | no | Key-value map passed to the target |


## Examples

### Hourly Docs Improvement

Run a docs-improve pipeline every hour to continuously catch and fix stale content.

```yaml
entries:
  - name: docs-improve
    schedule: "0 * * * *"
    kind: pipeline
    target: docs-improve
    input: themes documentation
    timeout: 15m
    working_dir: ~/projects/my-repo
```


### Docs Audit on the Half-Hour

Run a separate audit pass 30 minutes after each improvement cycle.

```yaml
entries:
  - name: docs-audit
    schedule: "30 * * * *"
    kind: pipeline
    target: sync-docs
    timeout: 10m
    working_dir: ~/projects/my-repo
```


### Nightly World Build

A heavier job that runs once a day at 5 AM — plenty of time to finish before you're back at your desk.

```yaml
entries:
  - name: nightly-build
    schedule: "0 5 * * *"
    kind: pipeline
    target: my-build-pipeline
    timeout: 45m
    working_dir: ~/projects/my-repo
```


### Full `cron.yaml` with Multiple Jobs

```yaml
entries:
  - name: docs-improve
    schedule: "0 * * * *"
    kind: pipeline
    target: docs-improve
    input: themes documentation
    timeout: 15m
    working_dir: ~/projects/my-repo

  - name: docs-audit
    schedule: "30 * * * *"
    kind: pipeline
    target: sync-docs
    timeout: 10m
    working_dir: ~/projects/my-repo

  - name: nightly-build
    schedule: "0 5 * * *"
    kind: pipeline
    target: my-build-pipeline
    timeout: 45m
    working_dir: ~/projects/my-repo
```


## See Also

- [Pipelines](/docs/pipelines/pipelines) — build the pipelines you schedule here
- [Brain](/docs/pipelines/brain) — add memory to scheduled pipelines so your assistant learns over time
- [Plugins](/docs/pipelines/plugins) — extend what your scheduled pipelines can do
