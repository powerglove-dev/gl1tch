---
title: "Workflows"
description: "Sequence pipelines and agents with runtime branching decisions and automatic checkpointing."
order: 5
---

Workflows orchestrate multi-step operations by chaining pipelines and agents together, making decisions at runtime, and supporting resumable execution. A single pipeline runs one DAG in isolation; a workflow runs many pipelines, agents, or decision nodes in sequence—and can branch based on model output. When a step fails, the workflow checkpoints and can be resumed from that point without re-running earlier steps.


## Architecture

Workflows live in `~/.config/glitch/workflows/` as YAML files. The **orchestrator** (`internal/orchestrator/`) loads and executes them:

```
┌──────────────────────────────────────────┐
│  glitch workflow run/resume commands     │  cmd/workflow.go
│  (entry points)                          │
└──────────────┬───────────────────────────┘
               │
               ↓
┌──────────────────────────────────────────┐
│  ConductorRunner                         │  orchestrator/conductor.go
│  • executes steps in order               │  (runs the workflow state machine)
│  • publishes BUSD lifecycle events       │
│  • checkpoints after every step          │
└──────────────┬───────────────────────────┘
               │
       ┌───────┼────────────┬──────────────┐
       ↓       ↓            ↓              ↓
   Pipeline  Agent-Ref   Decision      Parallel
   Dispatch  Pipeline    Node          Executor
   ──────────────────────────────────────────
   (all steps read from and write to shared WorkflowContext)
```

**Data flow**: CLI invokes `ConductorRunner.Run()` with a `WorkflowDef` and input. The runner executes steps sequentially, maintaining a shared `WorkflowContext` (key-value store) that persists each step's output. Decision nodes evaluate a prompt against Ollama and choose a branch. Parallel steps fan out via goroutines, then rejoin. After every step, the runner checkpoints the entire state to `workflow_checkpoints` table. BUSD events (`workflow.run.*`, `workflow.step.*`) stream to the activity feed.


## Technologies

| Tool | Purpose |
|------|---------|
| **Ollama** | Local LLM for decision node branching. Only LLM in the hot path (format: JSON, no freeform parsing). |
| **YAML** | Declarative workflow definitions; loaded into `WorkflowDef` struct with strict validation. |
| **BUSD** | Publish workflow and step lifecycle events; streams to activity feed. |
| **SQLite** | Checkpoint tables (`workflow_runs`, `workflow_checkpoints`) for resumable execution. |
| **OpenTelemetry** | Tracing and metrics for workflow runs and step duration. |


## Concepts

### WorkflowDef

A workflow definition is a YAML file with a name, version, and ordered list of steps:

```yaml
name: triage-and-fix
version: "1.0"
steps:
  - id: fetch-logs
    type: pipeline-ref
    pipeline: fetch-error-logs
  - id: analyze
    type: agent-ref
    agent: log-analyzer
  - id: decide-fix
    type: decision
    # ...
```

The `name` field is the workflow identifier used in CLI commands. The `version` field is semantic versioning for your own tracking (not enforced by the runner).


### Step Types

| Type | Purpose | Resolves to | Notes |
|------|---------|-------------|-------|
| `pipeline-ref` | Execute a pipeline from `~/.config/glitch/pipelines/` | `<pipeline>.pipeline.yaml` | Input expanded via `{{ctx.*}}`; output stored as `<step_id>.output` |
| `agent-ref` | Execute an APM agent pipeline | `apm.<agent>.pipeline.yaml` | Agent must be installed; same output behavior as pipeline-ref |
| `decision` | Evaluate a prompt with Ollama; branch execution | N/A | Returns a branch name; routes to `On.<branch>` step via step `id` lookup |
| `parallel` | Run multiple step sequences concurrently | N/A | All branches complete before next sequential step runs |


### WorkflowContext

A thread-safe key-value store shared across all steps in a workflow run. Each step can read outputs from prior steps and contribute its own output:

- **Keys**: Follow ADK-style scoping: `temp.<key>` for ephemeral per-run data (e.g., `temp.input` for CLI input); `<step_id>.output` for step outputs (auto-populated by dispatcher).
- **Values**: Strings; individual values exceeding 16 KB are truncated with a warning.
- **Expansion**: Template syntax `{{ctx.<key>}}` in step `input`, `prompt`, or pipeline `vars` expands to the stored value.

Example:
```
Step 1 (fetch-logs) produces output → stored as fetch-logs.output
Step 2 (analyze) reads {{ctx.fetch-logs.output}} in its input field
Step 2 produces output → stored as analyze.output
Step 3 (decide-fix) can access both via {{ctx.fetch-logs.output}} and {{ctx.analyze.output}}
```


### ConductorRunner

Executes a `WorkflowDef` from start to finish (or from a checkpoint during resume). Responsibilities:

1. **Sequential execution**: Runs steps in declaration order.
2. **Branching**: Decision nodes call `DecisionNode.Evaluate()`, which prompts Ollama. The returned branch name routes the next step via ID lookup in the `On` map.
3. **Parallel fan-out**: Parallel steps spawn goroutines for each branch; waits for all to complete.
4. **Checkpointing**: After every step (success or failure), writes a checkpoint record containing step ID, status, output, and full `WorkflowContext` snapshot.
5. **BUSD events**: Publishes `workflow.run.started`, `workflow.step.started`, `workflow.step.done` (or `.failed`), and `workflow.run.completed` (or `.failed`).
6. **Context threading**: Maintains a `WorkflowContext` across all steps; each step reads and writes to it.


### DecisionNode

Evaluates a Prompt against a local Ollama model and returns a branch name. The model is called with `format: "json"` and must return a JSON object with a `branch` field:

```json
{"branch": "yes"}
```

If Ollama is unavailable, the decision fails—but a `default_branch` field can soft-fail to a fallback step. Timeout is configurable (default 30 seconds).


## Configuration

### Workflow File Location

Workflow files live in `~/.config/glitch/workflows/` as `.workflow.yaml` files:

```
~/.config/glitch/workflows/
├── backup-and-verify.workflow.yaml
├── error-triage.workflow.yaml
└── morning-checklist.workflow.yaml
```

The filename (minus the `.workflow.yaml` suffix) is the workflow name used in CLI commands.


### Workflow YAML Schema

```yaml
name: <string, required>
  # Unique workflow identifier. Used in CLI commands and logging.
  # Example: "triage-and-fix", "morning-checklist"

version: <string, required>
  # Semantic version (e.g., "1.0", "1.1-beta").
  # Informational; not enforced by the runner.

steps: <array of WorkflowStep, required>
  # Ordered list of steps. Executed sequentially unless branching or parallel.
```

### WorkflowStep Schema

Each element in `steps` is a `WorkflowStep`:

```yaml
id: <string, required>
  # Unique step identifier within the workflow. Used in context references,
  # decision routing, and checkpoints. No spaces or special characters.

type: <string, required>
  # One of: pipeline-ref, agent-ref, decision, parallel

# For pipeline-ref and agent-ref:
pipeline: <string>
  # Pipeline name (without .pipeline.yaml extension).
  # Example: "fetch-error-logs"

agent: <string>
  # Agent name. Resolves to apm.<agent>.pipeline.yaml.
  # Example: "log-analyzer"

input: <string, optional>
  # String input to the pipeline. Can contain {{ctx.<key>}} expansions.
  # Example: "Error log: {{ctx.fetch-logs.output}}"

vars: <map[string]string, optional>
  # Pipeline variables (same as pipeline-level vars).
  # Each value can contain {{ctx.<key>}} expansions.
  # Example:
  #   timeout: "30"
  #   model: "llama2"

# For decision:
model: <string, required>
  # Ollama model name (e.g., "llama2", "neural-chat").

prompt: <string, required>
  # Prompt template sent to Ollama. Can contain {{ctx.<key>}} expansions.
  # The model must respond with JSON: {"branch": "<name>"}

on: <map[string]string, required>
  # Branch routing. Keys are branch names returned by the model.
  # Values are step IDs to execute next.
  # Example:
  #   "yes": "apply-fix"
  #   "no": "log-and-skip"

default_branch: <string, optional>
  # Fallback step ID if Ollama returns an error or unrecognized branch.
  # If omitted and an error occurs, the workflow fails.

timeout_secs: <int, optional>
  # Seconds to wait for Ollama to respond. Default: 30.

# For parallel:
branches: <array of ParallelBranch, required>
  # Concurrent branches. Each branch is a list of steps.
```

### ParallelBranch Schema

```yaml
steps: <array of WorkflowStep, required>
  # Ordered steps within this branch.
  # Can include any step type (pipeline-ref, agent-ref, decision, parallel).
```


## Examples

### Simple Linear Workflow

Three steps executed sequentially:

```yaml
name: backup-and-verify
version: "1.0"

steps:
  - id: backup
    type: pipeline-ref
    pipeline: daily-backup
    input: "target=database"

  - id: verify
    type: pipeline-ref
    pipeline: verify-backup
    input: "backup_path={{ctx.backup.output}}"

  - id: notify
    type: agent-ref
    agent: slack-notifier
    input: "status=complete backup_path={{ctx.backup.output}}"
```

Run with:
```bash
glitch workflow run backup-and-verify
```


### Workflow with Decision Branching

Fetch logs, analyze them, then decide whether to page on-call:

```yaml
name: error-triage
version: "1.0"

steps:
  - id: fetch-logs
    type: pipeline-ref
    pipeline: fetch-error-logs
    input: "hours=1"

  - id: analyze
    type: agent-ref
    agent: log-analyzer
    input: "logs={{ctx.fetch-logs.output}}"

  - id: assess-severity
    type: decision
    model: llama2
    prompt: |
      Based on these logs, is this a critical incident?
      Logs: {{ctx.analyze.output}}
      
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

Run with:
```bash
glitch workflow run error-triage
```


### Workflow with Parallel Branches

Fetch data from two sources concurrently, then merge:

```yaml
name: multi-source-gather
version: "1.0"

steps:
  - id: gather-parallel
    type: parallel
    branches:
      - steps:
          - id: fetch-s3
            type: pipeline-ref
            pipeline: s3-list-objects
            input: "bucket=my-logs"

          - id: parse-s3
            type: agent-ref
            agent: json-parser
            input: "data={{ctx.fetch-s3.output}}"

      - steps:
          - id: fetch-postgres
            type: pipeline-ref
            pipeline: query-postgres
            input: "query=SELECT * FROM events"

          - id: parse-postgres
            type: agent-ref
            agent: sql-to-json
            input: "data={{ctx.fetch-postgres.output}}"

  - id: merge
    type: pipeline-ref
    pipeline: merge-sources
    input: |
      s3={{ctx.parse-s3.output}}
      postgres={{ctx.parse-postgres.output}}
```

Run with:
```bash
glitch workflow run multi-source-gather
```


### Resuming a Failed Workflow

If a workflow fails, it checkpoints the state. Resume from the last failed step:

```bash
glitch workflow resume --run-id 42
```

This re-executes the failed step and continues from there, reusing all `WorkflowContext` data from prior successful steps.


## CLI Commands

### glitch workflow run

Run a workflow by name.

```bash
glitch workflow run <name> [--input "<string>"]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | Input string passed to the workflow as `temp.input`. Can be accessed in step `input` or `prompt` fields via `{{ctx.temp.input}}`. |

The workflow must exist at `~/.config/glitch/workflows/<name>.workflow.yaml`.


### glitch workflow resume

Resume a workflow run from its last checkpoint.

```bash
glitch workflow resume --run-id <id>
```

| Flag | Required | Description |
|------|----------|-------------|
| `--run-id` | yes | Workflow run ID from the checkpoint table. |

The runner restores the `WorkflowContext` from the checkpoint and re-executes the failed step, then continues to completion.


## When to Use Workflows vs. Pipelines

**Use a pipeline** if:
- You have a single, linear sequence of operations (fetch → analyze → report).
- All logic is self-contained and doesn't need to branch at runtime.
- You don't need to compose multiple independent operations.

**Use a workflow** if:
- You need to chain multiple pipelines or agents together.
- Execution must branch based on runtime decisions (model output, file contents, etc.).
- You want automatic checkpointing so transient failures don't lose progress.
- Steps need to share data (one step's output feeds into the next).
- You need parallel branches that run concurrently then rejoin.

In short: pipelines are for individual tasks; workflows orchestrate tasks into larger processes.


## See Also

- [Pipelines](/docs/pipelines) — single-DAG pipeline execution, the building block for workflow steps
- [CLI Reference](/docs/cli-reference) — `glitch workflow` command syntax
- [BUSD Architecture](/docs/busd) — event publishing and activity feed
- [Store and Checkpointing](/docs/store) — persistence and resumable execution

