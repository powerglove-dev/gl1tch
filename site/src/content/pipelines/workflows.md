---
title: "Workflows"
description: "Chain multiple pipelines into larger automations with branching and checkpointing."
order: 5
---

When one pipeline isn't enough, use a workflow. A workflow chains multiple pipelines together, passes output from one to the next, and can branch at runtime based on what an AI decides. If something fails mid-run, the workflow checkpoints and you resume from that step — not from the beginning.

## Quick Start

Create a workflow file, then run it:

```yaml
# ~/.config/glitch/workflows/morning-check.workflow.yaml
name: morning-check
version: "1.0"

steps:
  - id: fetch-prs
    type: pipeline-ref
    pipeline: list-open-prs

  - id: summarize
    type: pipeline-ref
    pipeline: daily-summary
    input: "prs={{ctx.fetch-prs.output}}"

  - id: notify
    type: pipeline-ref
    pipeline: send-slack
    input: "message={{ctx.summarize.output}}"
```

```bash
glitch workflow run morning-check
```

Output flows automatically: each step's result is stored under `<step-id>.output` and available to later steps as `{{ctx.<step-id>.output}}`.

## How Output Flows

Every step writes its output into a shared context. Later steps read it with `{{ctx.<step-id>.output}}`.

```text
Step 1 → output stored as "fetch-prs.output"
Step 2 → reads {{ctx.fetch-prs.output}} → output stored as "summarize.output"
Step 3 → reads {{ctx.summarize.output}}
```

No manual wiring. The context persists across the entire workflow run.

## Step Types

| Type | What it does |
|------|-------------|
| `pipeline-ref` | Runs a pipeline from `~/.config/glitch/pipelines/`. |
| `agent-ref` | Runs an installed agent pipeline. |
| `decision` | Asks a local model a question; branches execution based on the answer. |
| `parallel` | Runs multiple step sequences at the same time, then rejoins. |

## Decision Branching

A `decision` step asks your local model a question and routes execution based on the answer. The model must return JSON: `{"branch": "<name>"}`.

```yaml
name: error-triage
version: "1.0"

steps:
  - id: fetch-logs
    type: pipeline-ref
    pipeline: fetch-error-logs
    input: "hours=1"

  - id: analyze
    type: pipeline-ref
    pipeline: analyze-logs
    input: "logs={{ctx.fetch-logs.output}}"

  - id: assess-severity
    type: decision
    model: llama3.2
    prompt: |
      Are these logs showing a critical incident?
      {{ctx.analyze.output}}
      Respond with JSON: {"branch":"critical"} or {"branch":"normal"}
    on:
      critical: page-oncall
      normal: log-and-close
    default_branch: log-and-close
    timeout_secs: 30

  - id: page-oncall
    type: agent-ref
    agent: pagerduty
    input: "incident={{ctx.analyze.output}}"

  - id: log-and-close
    type: pipeline-ref
    pipeline: log-incident
    input: "severity=normal summary={{ctx.analyze.output}}"
```

```bash
glitch workflow run error-triage
```

## Parallel Branches

Run multiple pipelines at the same time, then continue once all branches finish:

```yaml
name: multi-source-gather
version: "1.0"

steps:
  - id: gather-parallel
    type: parallel
    branches:
      - steps:
          - id: fetch-github
            type: pipeline-ref
            pipeline: github-issues
            input: "repo=8op-org/gl1tch"

      - steps:
          - id: fetch-metrics
            type: pipeline-ref
            pipeline: pull-metrics
            input: "window=24h"

  - id: merge
    type: pipeline-ref
    pipeline: merge-report
    input: |
      github={{ctx.fetch-github.output}}
      metrics={{ctx.fetch-metrics.output}}
```

## Resuming After Failure

If a workflow step fails, the run checkpoints automatically. Resume it from the failed step:

```bash
glitch workflow resume --run-id 42
```

All context from successful earlier steps is preserved. The failed step re-runs; the rest continues from there.

## Workflow File Reference

Workflow files live in `~/.config/glitch/workflows/` with a `.workflow.yaml` extension. The filename (minus `.workflow.yaml`) is the name used in CLI commands.

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | yes | Workflow identifier. Used in CLI commands and logs. |
| `version` | `string` | yes | Informational version string, e.g. `"1.0"`. Not enforced. |
| `steps` | `Step[]` | yes | Ordered list of steps. |

### Step Fields

| Field | Type | Applies to | Description |
|-------|------|-----------|-------------|
| `id` | `string` | all | Unique step ID within the workflow. |
| `type` | `string` | all | One of: `pipeline-ref`, `agent-ref`, `decision`, `parallel`. |
| `pipeline` | `string` | `pipeline-ref` | Pipeline name (without `.pipeline.yaml`). |
| `agent` | `string` | `agent-ref` | Agent name. Resolves to `apm.<agent>.pipeline.yaml`. |
| `input` | `string` | `pipeline-ref`, `agent-ref` | Input string. Supports `{{ctx.<key>}}` expressions. |
| `vars` | `map[string]string` | `pipeline-ref`, `agent-ref` | Pipeline variables. Values support `{{ctx.<key>}}` expressions. |
| `model` | `string` | `decision` | Local model to use for the decision. |
| `prompt` | `string` | `decision` | Prompt sent to the model. Must instruct it to return `{"branch":"<name>"}`. |
| `on` | `map[string]string` | `decision` | Branch routing. Keys are branch names; values are step IDs. |
| `default_branch` | `string` | `decision` | Step ID to use if the model fails or returns an unknown branch. |
| `timeout_secs` | `int` | `decision` | Seconds to wait for the model. Default: `30`. |
| `branches` | `Branch[]` | `parallel` | List of concurrent step sequences. Each branch has its own `steps` array. |

## When to Use Workflows vs Pipelines

Use a **pipeline** when:
- You have one sequence of operations — fetch, analyze, report.
- Everything is self-contained.

Use a **workflow** when:
- You need to chain multiple pipelines together.
- Execution needs to branch based on what a model decides.
- You want automatic checkpointing so a failure doesn't lose your progress.
- Steps need to run in parallel then rejoin.

## See Also

- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — the building blocks of workflow steps
- [CLI Reference](/docs/pipelines/cli-reference) — `glitch workflow` command syntax
- [Executors](/docs/pipelines/executors) — what runs inside each pipeline step
