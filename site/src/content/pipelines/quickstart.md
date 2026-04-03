---
title: "Your First Pipeline"
description: "Install gl1tch and run your first AI-powered pipeline in under five minutes."
order: 1
---

gl1tch runs automations you write in YAML and executes them with real AI. This page gets you from zero to a working pipeline in under five minutes. No theory — just install, write, run.


## Quick Start

**Step 1 — Install:**

```bash
go install github.com/8op-org/gl1tch/cmd/glitch@latest
```

You need Go 1.22+ and either [Ollama](https://ollama.ai) running locally or the [Claude CLI](https://claude.ai/download) authenticated. No Docker, no cloud account required.

**Step 2 — Write a pipeline:**

Create `hello.pipeline.yaml` anywhere on disk:

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

**Step 3 — Run it:**

```bash
glitch pipeline run hello.pipeline.yaml
```

You'll see a sysop greeting stream to your terminal. That's your first pipeline.

> **TIP:** Using Ollama instead of Claude? Swap executor and model:
> ```yaml
> executor: ollama
> model: qwen2.5-coder:latest
> ```


## What Just Happened

gl1tch read your YAML, found the `greet` step, sent your prompt to Claude, and streamed the response to stdout. Every pipeline run is stored locally so you can review it later.

Single-step pipelines are useful for quick tasks. The real power comes from chaining steps together.


## Chaining Steps

Steps can pass their output to later steps. This pipeline fetches a GitHub repo description and summarizes it:

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

Two things to notice:

- `needs: [get-repo]` — the `summarize` step waits for `get-repo` to finish before it runs.
- `{{steps.get-repo.output}}` — injects the previous step's output into the prompt.

The `gh` executor wraps the [GitHub CLI](https://cli.github.com/). Make sure it's installed and authenticated before running this one.


## Where to Save Your Pipelines

`glitch pipeline run` accepts a file path or a pipeline name:

```bash
# Run by file path (good for development)
glitch pipeline run ./hello.pipeline.yaml

# Run by name (looks in ~/.config/glitch/pipelines/)
glitch pipeline run hello
```

Drop pipelines you use regularly into `~/.config/glitch/pipelines/`. They show up automatically in your gl1tch workspace.


## Next Steps

- [Pipelines](/docs/pipelines/pipelines) — Full guide to writing and structuring pipelines
- [Console](/docs/pipelines/console) — Your gl1tch workspace: chat, launch, inspect runs
- [Examples](/docs/pipelines/examples) — Copy-paste pipelines for real developer workflows


## See Also

- [Pipelines](/docs/pipelines/pipelines)
- [Console](/docs/pipelines/console)
- [Examples](/docs/pipelines/examples)
