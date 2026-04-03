---
title: "Cron Scheduling"
description: "Schedule your pipelines to run automatically — wake up to results already waiting in your workspace."
order: 9
---

Set a pipeline on a schedule and stop thinking about it. Your morning standup prep runs at 8 AM. Your nightly code review finishes while you sleep. Your dependency audit fires every Monday. gl1tch runs your pipelines on a cron schedule and logs the results — you just check in when you're ready.


## Quick Start

Create `~/.config/glitch/cron.yaml` with your first scheduled pipeline:

```yaml
entries:
  - name: morning-standup
    schedule: "0 8 * * 1-5"
    kind: pipeline
    target: ~/pipelines/standup.pipeline.yaml
    timeout: "5m"
```

Start the scheduler:

```bash
glitch cron start
```

Check that it registered:

```bash
glitch cron list
```

```text
morning-standup   0 8 * * 1-5   pipeline   standup.pipeline.yaml   in 14h 22m
```

Your pipeline now runs every weekday at 8 AM without you touching it.


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
| `30 17 * * 5` | 5:30 PM every Friday |

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

```text
morning-standup   0 8 * * 1-5    pipeline   standup.pipeline.yaml     in 14h 22m
nightly-review    0 23 * * *     pipeline   code-review.pipeline.yaml  in 6h 44m
dep-audit         0 9 * * 1      pipeline   deps.pipeline.yaml         in 3d 2h
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
    target: <string>        # path to .pipeline.yaml, or agent name
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
| `target` | yes | Path to a `.pipeline.yaml` or an agent name |
| `timeout` | no | Max run duration — e.g. `"10m"`, `"30s"` |
| `working_dir` | no | Directory the job runs in; inherits your workspace if empty |
| `input` | no | String mapped to `{{param.input}}` inside the pipeline |
| `args` | no | Key-value map passed to the target |


## Examples


### Morning Standup Brief

Your assistant pulls open PRs and recent commits, summarizes the day ahead, and has it ready before you open your laptop.

```yaml
entries:
  - name: morning-standup
    schedule: "0 8 * * 1-5"
    kind: pipeline
    target: ~/pipelines/standup.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "5m"
```


### Nightly Code Review

Run a full code quality check every night. Wake up to findings already in your log.

```yaml
entries:
  - name: nightly-review
    schedule: "0 23 * * *"
    kind: pipeline
    target: ~/pipelines/code-review.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "15m"
```


### Weekly Dependency Audit

Every Monday morning, check for outdated or vulnerable dependencies before the week starts.

```yaml
entries:
  - name: dep-audit
    schedule: "0 9 * * 1"
    kind: pipeline
    target: ~/pipelines/dep-check.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "10m"
```


### Full `cron.yaml` with Multiple Jobs

```yaml
entries:
  - name: morning-standup
    schedule: "0 8 * * 1-5"
    kind: pipeline
    target: ~/pipelines/standup.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "5m"

  - name: nightly-review
    schedule: "0 23 * * *"
    kind: pipeline
    target: ~/pipelines/code-review.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "15m"

  - name: dep-audit
    schedule: "0 9 * * 1"
    kind: pipeline
    target: ~/pipelines/dep-check.pipeline.yaml
    working_dir: ~/projects/my-repo
    timeout: "10m"
```


## See Also

- [Pipelines](/docs/pipelines/pipelines) — build the pipelines you schedule here
- [Brain](/docs/pipelines/brain) — add memory to scheduled pipelines so your assistant learns over time
- [Plugins](/docs/pipelines/plugins) — extend what your scheduled pipelines can do
