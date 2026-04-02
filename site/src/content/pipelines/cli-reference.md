---
title: "CLI Reference"
description: "Every glitch command — ask, pipeline, cron, config, and more."
order: 6
---

The `glitch` CLI is the primary interface for running pipelines, managing configuration, and interacting with the AI layer from the terminal.


## glitch ask

Send a prompt to glitch from the terminal. Routes to a matching pipeline automatically, or generates one on the fly if nothing matches.

```bash
glitch ask "sync my docs with the latest code changes"
glitch ask "what PRs need my review"
glitch ask "what's on my calendar tomorrow"
```

Defaults to the first available local provider (ollama). No remote API calls unless you ask for them.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-p`, `--provider` | *(local)* | Provider ID to use: `ollama`, `claude`, `opencode`, etc. |
| `-m`, `--model` | *(provider default)* | Model name, e.g. `llama3.2`, `mistral`. |
| `--pipeline` | *(none)* | Run a named pipeline or file path instead of routing. |
| `--input key=value` | *(none)* | Pass vars into the pipeline. Repeatable. |
| `--brain` | `true` | Inject brain context from the store to ground the response. |
| `--write-brain` | `false` | Write the response back to the brain store. |
| `--synthesize` | `false` | Run response through claude to clean it up without adding new information. |
| `--synthesize-model` | *(claude default)* | Model used for the synthesis pass. |
| `--json` | `false` | Output the response as a JSON envelope. |
| `--route` | `true` | Attempt to route the prompt to a matching pipeline automatically. |
| `--auto`, `-y` | `false` | Skip confirmation when a pipeline is generated on the fly. |
| `--dry-run` | `false` | Show what would run without executing. |


## glitch pipeline

Run and manage pipelines.

### glitch pipeline run

```bash
glitch pipeline run <name|file>
glitch pipeline run sync-docs
glitch pipeline run ./my-pipeline.yaml --input "focus on auth changes"
```

Looks up `<name>` as `~/.config/glitch/pipelines/<name>.pipeline.yaml`. Pass a path directly to use any file on disk.

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | User input string passed to the pipeline as `{{param.input}}`. |

### glitch pipeline resume

Resume a pipeline that paused waiting for a clarification answer.

```bash
glitch pipeline resume --run-id <id>
```

The run ID is shown in the TUI inbox when a pipeline is paused.

| Flag | Default | Description |
|------|---------|-------------|
| `--run-id` | *(required)* | Store run ID to resume. |


## glitch cron

Schedule pipelines to run automatically.

```bash
glitch cron start           # Start the cron daemon in a background tmux session
glitch cron stop            # Stop the daemon
glitch cron list            # List scheduled jobs
glitch cron logs            # View recent cron run logs
glitch cron run <name>      # Run a cron job manually right now
```

The cron daemon runs pipelines on a schedule defined in `~/.config/glitch/cron.yaml`. It runs in a detached tmux session named `glitch-cron`.

### glitch cron start

```bash
glitch cron start
glitch cron start --force   # Kill existing session and restart
```

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Kill an existing cron session before starting. |


## glitch config

Manage configuration files.

### glitch config init

Generates the default configuration files if they don't exist yet:

```bash
glitch config init
```

Creates:
- `~/.config/glitch/layout.yaml` — pane layout for the TUI
- `~/.config/glitch/keybindings.yaml` — keyboard shortcut overrides


## glitch backup

Back up your config, pipelines, prompts, and brain data to a portable archive.

```bash
glitch backup
glitch backup --output ./my-backup.tar.gz
```

Produces a `.tar.gz` file containing all config files from `~/.config/glitch/` and an exported JSONL snapshot of brain notes and saved prompts from the database.

| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `glitch-backup-<date>.tar.gz` | Output path for the backup archive. |
| `--no-agents` | `false` | Exclude auto-generated agent pipelines from `pipelines/.agents/`. |


## glitch restore

Restore config and brain data from a backup archive.

```bash
glitch restore ./glitch-backup-2025-01-15.tar.gz
glitch restore ./backup.tar.gz --overwrite
glitch restore ./backup.tar.gz --dry-run
```

Merges config files back into `~/.config/glitch/` and re-imports DB records. Skips existing files by default.

| Flag | Default | Description |
|------|---------|-------------|
| `--overwrite` | `false` | Overwrite existing config files on conflict. |
| `--dry-run` | `false` | Preview changes without writing anything. |


## glitch plugin

Manage installed plugins.

### glitch plugin install

```bash
glitch plugin install owner/repo
glitch plugin install owner/repo@v1.2.3
```

Downloads and installs a plugin from a GitHub repository. Writes the plugin binary and registers a sidecar wrapper so pipelines can use it immediately.

### glitch plugin remove

```bash
glitch plugin remove <name>
glitch plugin rm <name>
```

Removes an installed plugin.

### glitch plugin list

```bash
glitch plugin list
glitch plugin ls
```

Lists all installed plugins with their sources and binary paths.


## glitch workflow

Run and manage multi-step workflows.

### glitch workflow run

```bash
glitch workflow run <name>
glitch workflow run my-workflow --input "context here"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | Input string passed to the workflow as `temp.input`. |

### glitch workflow resume

```bash
glitch workflow resume --run-id <id>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--run-id` | *(required)* | Workflow run ID to resume. |


## glitch widget

Standalone TUI widget subcommands.

### glitch widget jump-window

```bash
glitch widget jump-window
```

Opens the jump window TUI as a standalone process. Useful for launching it directly from a keybinding without starting the full glitch session.


## See Also

- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — every field and what it does
- [Executors and Plugins](/docs/pipelines/executors) — what runs your steps
- [The Brain System](/docs/pipelines/brain) — persistent context across steps
