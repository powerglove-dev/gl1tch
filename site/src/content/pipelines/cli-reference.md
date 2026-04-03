---
title: "CLI Reference"
description: "Look up every glitch command, subcommand, and flag."
order: 6
---

Complete command reference for the `glitch` CLI. Every subcommand and flag is listed below. Use Ctrl+F to find what you need.

## glitch ask

| Flag | Default | Description |
|------|---------|-------------|
| `-p`, `--provider` | *(local)* | Provider to use: `ollama`, `claude`, `opencode`, etc. |
| `-m`, `--model` | *(provider default)* | Model name, e.g. `llama3.2`, `mistral`. |
| `--pipeline` | *(none)* | Run a named pipeline or file path instead of routing. |
| `--input key=value` | *(none)* | Pass vars into the pipeline. Repeatable. |
| `--brain` | `true` | Inject brain context to ground the response. |
| `--write-brain` | `false` | Write the response back to the brain store. |
| `--synthesize` | `false` | Run the response through a cleanup pass without adding new information. |
| `--synthesize-model` | *(default)* | Model used for the synthesis pass. |
| `--json` | `false` | Output the response as a JSON envelope. |
| `--route` | `true` | Route the prompt to a matching pipeline automatically. |
| `--auto`, `-y` | `false` | Skip confirmation when a pipeline is generated on the fly. |
| `--dry-run` | `false` | Show what would run without executing. |

```bash
glitch ask "sync my docs with the latest code changes"
glitch ask "what PRs need my review" --provider claude
glitch ask "summarize this" --pipeline ./my-pipeline.yaml
```

---

## glitch pipeline

### glitch pipeline run

Run a pipeline by name or file path.

```bash
glitch pipeline run <name|file>
glitch pipeline run sync-docs
glitch pipeline run ./my-pipeline.yaml --input "focus on auth changes"
```

Looks up `<name>` as `~/.config/glitch/pipelines/<name>.pipeline.yaml`.

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | User input string, available as `{{param.input}}` in the pipeline. |

### glitch pipeline resume

Resume a pipeline that paused waiting for a clarification.

```bash
glitch pipeline resume --run-id <id>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--run-id` | *required* | Store run ID to resume. Shown in the inbox when a pipeline is paused. |

---

## glitch workflow

### glitch workflow run

Run a workflow by name.

```bash
glitch workflow run <name>
glitch workflow run my-workflow --input "context here"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | Input string passed to the workflow as `temp.input`. |

### glitch workflow resume

Resume a workflow run from its last checkpoint.

```bash
glitch workflow resume --run-id <id>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--run-id` | *required* | Workflow run ID to resume. |

---

## glitch cron

Schedule pipelines to run automatically.

```bash
glitch cron start           # Start the scheduler in a background session
glitch cron stop            # Stop the scheduler
glitch cron list            # List scheduled jobs
glitch cron logs            # View recent cron run logs
glitch cron run <name>      # Run a cron job manually right now
```

### glitch cron start

```bash
glitch cron start
glitch cron start --force
```

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Kill an existing cron session before starting. |

---

## glitch config

### glitch config init

Generate default configuration files if they don't exist yet.

```bash
glitch config init
```

Creates:
- `~/.config/glitch/layout.yaml` — your workspace layout
- `~/.config/glitch/keybindings.yaml` — keyboard shortcut overrides

---

## glitch plugin

### glitch plugin install

```bash
glitch plugin install owner/repo
glitch plugin install owner/repo@v1.2.3
```

Downloads and installs a plugin from GitHub. Registers a wrapper so pipelines can use it immediately.

### glitch plugin remove

```bash
glitch plugin remove <name>
glitch plugin rm <name>
```

### glitch plugin list

```bash
glitch plugin list
glitch plugin ls
```

Lists installed plugins with their sources and binary paths.

---

## glitch backup

Back up your config, pipelines, prompts, and brain data.

```bash
glitch backup
glitch backup --output ./my-backup.tar.gz
```

| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `glitch-backup-<date>.tar.gz` | Output path for the backup archive. |
| `--no-agents` | `false` | Exclude auto-generated agent pipelines from `pipelines/.agents/`. |

---

## glitch restore

Restore config and brain data from a backup archive.

```bash
glitch restore ./glitch-backup-2025-01-15.tar.gz
glitch restore ./backup.tar.gz --overwrite
glitch restore ./backup.tar.gz --dry-run
```

| Flag | Default | Description |
|------|---------|-------------|
| `--overwrite` | `false` | Overwrite existing config files on conflict. |
| `--dry-run` | `false` | Preview changes without writing anything. |

---

## glitch widget

### glitch widget jump-window

Open the jump window as a standalone process. Useful for launching directly from a keybinding without starting a full session.

```bash
glitch widget jump-window
```

---

## See Also

- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — every field and what it does
- [Executors](/docs/pipelines/executors) — what runs your steps
- [Workflows](/docs/pipelines/workflows) — chain multiple pipelines together
