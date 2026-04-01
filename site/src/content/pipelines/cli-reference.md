---
title: "CLI Reference"
description: "Every glitch command — ask, pipeline, cron, config, and more."
order: 6
---

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
| `--route` | `true` | Attempt to route the prompt to a matching pipeline automatically. |
| `--auto`, `-y` | `false` | Skip confirmation when a pipeline is generated on the fly. |
| `--dry-run` | `false` | Show which pipeline would run (or the generated YAML) without executing. |
| `--json` | `false` | Output the response as a JSON envelope. |

### How routing works

When `--route` is on (the default), `glitch ask` scans `~/.config/glitch/pipelines/` and uses a local model to classify your prompt against the available pipelines. If a match is found, that pipeline runs with your prompt as its input. If nothing matches, a pipeline is generated on the fly and you're asked to confirm before it runs.

Add a `description:` field to your pipeline YAMLs to give the router better signal:

```yaml
name: sync-docs
description: "Compare recent code changes against site docs and generate a sync report"
version: "1"
steps:
  ...
```

To skip routing entirely and get a direct one-shot response:

```bash
glitch ask --route=false "what does write_brain do?"
```

### Self-improvement loop

Combine `--brain` and `--write-brain` to build context over time:

```bash
glitch ask --brain --write-brain "expand on how the pipeline retry system works"
```

Each run reads existing brain context, generates a grounded response, and writes it back. Later runs are richer because earlier runs accumulated knowledge.

---

## glitch pipeline

Run and manage pipelines.

### glitch pipeline run

```bash
glitch pipeline run <name|file>
glitch pipeline run sync-docs
glitch pipeline run ./my-pipeline.yaml --input "focus on auth changes"
```

Looks up `<name>` as `~/.config/glitch/pipelines/<name>.pipeline.yaml`. Pass a path directly to use any file on disk.

### glitch pipeline resume

Resume a pipeline that paused waiting for a clarification answer.

```bash
glitch pipeline resume --run-id <id>
```

The run ID is shown in the TUI inbox when a pipeline is paused.

---

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

---

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
