---
title: "Architecture"
description: "Trace what happens when you run a pipeline — from command to output."
order: 10
---

gl1tch is a single binary that runs in your terminal. Here's what happens from the moment you type `glitch pipeline run` to the moment you see output.

## What Happens When You Run a Pipeline

```text
You type:
  glitch pipeline run code-review

         │
         ▼
  gl1tch reads your pipeline YAML
  ~/.config/glitch/pipelines/code-review.pipeline.yaml

         │
         ▼
  The runner builds a dependency graph from your steps
  (which steps depend on which other steps)

         │
         ▼
  Steps run in order — parallel where possible
  Each step:
    1. Resolves template expressions ({{steps.x.output}})
    2. Calls the executor (ollama, claude, gh, etc.)
    3. Captures the output
    4. Stores it locally

         │
         ▼
  Results appear in your terminal
  Full run history written to ~/.local/share/glitch/glitch.db
```

That's it. No hidden agents. No cloud orchestration. The work happens on your machine.

## The Control Panel

When you run `glitch` without a subcommand, you get a three-column workspace:

```text
┌─────────────────┬──────────────────┬──────────────────┐
│  Your Pipelines │  gl1tch Chat     │  Activity Feed   │
│  ────────────── │  ──────────────  │  ──────────────  │
│                 │                  │                  │
│  list of all    │  Ask questions,  │  Live output     │
│  your pipelines │  get answers,    │  from running    │
│  (searchable)   │  launch jobs     │  pipelines       │
│                 │                  │                  │
│  Agent Runner   │  Agent Grid      │  Step statuses   │
│  (run a model   │  Signal Board    │  Elapsed time    │
│   with a prompt)│  Inbox Detail    │  Results         │
└─────────────────┴──────────────────┴──────────────────┘
```

**Left column:** Your pipeline list and agent runner form. Tab cycles focus between them.

**Center column:** The main interaction area. Chat with your assistant, monitor running jobs, review past results.

**Right column:** The activity feed. Every pipeline run streams here in real time — step by step, with status and timing.

## Where Things Live

| What | Where |
|------|-------|
| Your pipelines | `~/.config/glitch/pipelines/` |
| Your workflows | `~/.config/glitch/workflows/` |
| Your tool wrappers | `~/.config/glitch/wrappers/` |
| Your themes | `~/.config/glitch/themes/` |
| Run history (SQLite) | `~/.local/share/glitch/glitch.db` |
| Trace logs | `~/.local/share/glitch/traces.jsonl` |

Everything is on your disk. Nothing requires a network connection for core operation.

## How the Brain Works

The brain is a local database of context from your pipeline runs. When a step has `write_brain: true`, its output is indexed and stored. On future runs with `--brain`, gl1tch retrieves relevant context and injects it into your prompts.

Your workspace learns over time without you having to re-explain things.

## How Scheduling Works

Your workspace can run pipelines on a schedule. The scheduler watches `~/.config/glitch/pipelines/` and fires pipelines based on their `cron` field:

```yaml
name: morning-summary
cron: "0 8 * * 1-5"   # 8am, Monday–Friday
steps:
  ...
```

Start the scheduler:

```bash
glitch cron start
```

The scheduler runs in a detached terminal session and keeps going whether you're watching or not.

## How glitch ask Works

When you run `glitch ask "summarize my open PRs"`, gl1tch:

1. Routes your prompt to a matching pipeline using your local model.
2. If a match is found, runs that pipeline with your prompt as input.
3. If no match is found, either generates a pipeline on the fly (with your confirmation) or responds directly.

The routing decision stays local. Cloud models are only called if the matched pipeline explicitly uses one.

## See Also

- [Philosophy](/docs/pipelines/philosophy) — why gl1tch works the way it does
- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — writing your first pipeline
- [Executors](/docs/pipelines/executors) — what runs inside each step
- [CLI Reference](/docs/pipelines/cli-reference) — every command and flag
