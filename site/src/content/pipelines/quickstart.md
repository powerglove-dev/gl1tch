---
title: "Your First Pipeline"
description: "Install gl1tch, write a pipeline YAML, run it. Five minutes."
order: 1
---

## Prerequisites

You need two things:

- **Go 1.22+** installed on your machine. Run `go version` to check.
- **An AI provider**: either the [Claude CLI](https://claude.ai/download) installed and authenticated, or [Ollama](https://ollama.ai) running locally with a model pulled.

That's it. No Docker, no cloud account, no config ceremony.

## Install

From source (recommended for now):

```bash
git clone https://github.com/8op-org/gl1tch.git
cd gl1tch
go build ./cmd/glitch
```

Or grab it directly:

```bash
go install github.com/8op-org/gl1tch/cmd/glitch@latest
```

Either way you end up with a `glitch` binary. Put it somewhere on your `$PATH`.

## Your first pipeline

Create a file called `hello.pipeline.yaml`. Anywhere on disk is fine.

```yaml
name: hello
version: "1"
steps:
  - id: greet
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: |
      Say hello in the style of a 1990s BBS sysop. Keep it under 3 lines.
```

Run it:

```bash
glitch pipeline run hello.pipeline.yaml
```

The runner builds the executor, sends the prompt, and streams output to stdout. You should see something like a sysop greeting from 1994 appear in your terminal.

If you're using Ollama instead of Claude, swap the executor and model:

```yaml
- id: greet
  executor: ollama
  model: qwen2.5-coder:latest
  prompt: |
    Say hello in the style of a 1990s BBS sysop. Keep it under 3 lines.
```

## What just happened

When you run `glitch pipeline run`, the runner:

1. Parses the YAML and validates the step DAG
2. Resolves each step's executor (native like `claude`/`ollama`, or sidecar CLI wrappers loaded from `~/.config/glitch/wrappers/`)
3. Runs steps in dependency order, streaming output as it goes
4. Stores the run result in the local SQLite database

Single-step pipelines are fine for testing, but the real power is chaining.

## Chaining steps

Here's a two-step pipeline that fetches a repo description from GitHub and summarizes it:

```yaml
name: git-summary
version: "1"
steps:
  - id: get-repo
    executor: gh
    vars:
      args: "repo view --json description"

  - id: summarize
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [get-repo]
    prompt: |
      Summarize this repo description in one sentence:
      {{steps.get-repo.output}}
```

The `needs` field creates a dependency. `summarize` won't run until `get-repo` finishes. The `{{steps.get-repo.output}}` template expression injects the previous step's output into the prompt.

To use the `gh` executor, you need the [GitHub CLI](https://cli.github.com/) installed and authenticated. The `gh` executor is a sidecar wrapper -- a YAML file in `~/.config/glitch/wrappers/` that tells gl1tch how to call the `gh` command.

## Where pipelines live

The `glitch pipeline run` command accepts:

- A **file path**: `glitch pipeline run ./my-pipeline.pipeline.yaml`
- A **pipeline name**: `glitch pipeline run my-pipeline` -- this looks in `~/.config/glitch/pipelines/my-pipeline.pipeline.yaml`

For development, use file paths. For pipelines you run often, drop them in the config directory.

## Next steps

- Read the [YAML Reference](/docs/pipelines/yaml-reference) for every field you can use
- Learn about [Executors and Plugins](/docs/pipelines/executors) to understand what runs your steps
- Explore the [Brain System](/docs/pipelines/brain) for persistent context across steps
- Copy a [Real-World Pipeline](/docs/pipelines/examples) and adapt it
