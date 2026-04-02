---
title: "Workflows"
description: "Sequence pipelines and agents with runtime branching decisions and automatic checkpointing."
order: 5
---

Workflows orchestrate multi-step operations by chaining pipelines and agents together, making decisions at runtime, and supporting resumable execution. A single pipeline runs one isolated DAG; a workflow runs many pipelines, agents, or decision nodes in sequence and can branch based on model output. When a step fails, the workflow checkpoints and can be resumed from that point without re-running earlier steps.


## Architecture

Workflows live in `~/.config/glitch/workflows/` as YAML files. The **orchestrator** (`internal/orchestrator/`) loads and executes them. The conductor — the core execution engine — runs steps in order, calls Ollama for branching decisions, maintains shared context, and checkpoints state at every step boundary.

```
┌──────────────────────────────────────────┐
│  glitch workflow run/resume              │ — cmd/workflow.go
│  (CLI entry points)                      │
└──────────────┬───────────────────────────┘
               │
               ↓
┌──────────────────────────────────────────┐
│  ConductorRunner                         │ — conductor.go
│  • executes steps in declaration order   │
│  • invokes decision nodes for branches   │
│  • publishes BUSD lifecycle events       │
│  • checkpoints after every step          │
└──────────────┬───────────────────────────┘
               │
       ┌───────┴──────────┬────────────────┐
       ↓                  ↓                 ↓
  StepDispatcher    DecisionNode      WorkflowContext
  • resolves &      • calls Ollama    • thread-safe
    runs pipeline/    with format:json  key-value store
    agent steps     • extracts branch  • {{ctx.*}} template
  • stores output     name              expansion
    in context                         • max 16 KB per key
```

Data flows left to right: the conductor dispatches steps (which resolve pipeline files and execute them), calls Ollama for decisions, and maintains context across the workflow. After each step completes, the conductor writes a checkpoint to SQLite containing the step's status, output, and full serialized context. If a step fails, you can resume from the last checkpoint without replaying earlier steps.


## Technologies

- **Ollama** — Local LLM used for decision nodes. The conductor calls `POST /api/generate` with `format:json` to get structured branch decisions. Configurable via `--ollama-url`; defaults to `http://localhost:11434`. If Ollama is unavailable, the decision node can fall back to a `default_branch`.
- **SQLite** — Stores `workflow_runs` and `workflow_checkpoints` tables for persistence and resumability. Each checkpoint captures the full `WorkflowContext` as JSON.
- **BUSD** — The bus-driven event system publishes `workflow.run.*` (started, completed, failed) and `workflow.step.*` (started, done, failed) events so the Switchboard can track progress and the activity feed shows live updates.


## Concepts

### WorkflowDef

A workflow definition is a YAML file with a name, version, and ordered list of steps. The definition is immutable after loading; execution is driven by the `ConductorRunner` which maintains separate runtime state (the `WorkflowContext` and checkpoints).

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
    model: mistral
    prompt: "Should we auto-fix? {{ctx.analyze.output}}"
    on:
      yes: apply-fix
      no: notify-team
```

### Step Types and Execution

The conductor supports four step types, each with distinct behavior:

| Type | Purpose | Resolution | Next Step |
|------|---------|-----------|-----------|
| `pipeline-ref` | Run a standalone pipeline | `~/.config/glitch/pipelines/<pipeline>.pipeline.yaml` | Sequential; execution continues to next step |
| `agent-ref` | Run an APM agent as a step | `~/.config/glitch/pipelines/apm.<agent>.pipeline.yaml` | Sequential; requires agent to be installed |
| `decision` | Query Ollama and branch | Prompt is evaluated against local Ollama | Execution routes to the step named in the model's JSON `branch` field |
| `parallel` | Run branches concurrently | Multiple `WorkflowStep` lists execute simultaneously | Workflow waits for all branches to complete, then continues |

**Sequential execution**: Steps run in declaration order. After step `fetch-logs` completes, its output is stored in the context as `fetch-logs.output`, available to all following steps.

**Branching**: A `decision` step sends a prompt to Ollama and expects a JSON response with a `branch` field. The value routes execution to a different step — steps not on the path are skipped. If Ollama fails or the response is malformed, the workflow branches to `default_branch` (or fails if no default is set).

**Parallel execution**: A `parallel` step runs multiple branches concurrently. All branches share the same `WorkflowContext`, so each branch can read outputs from earlier sequential steps. Outputs from each branch are stored as usual (`<step_id>.output`). The workflow waits for all branches to complete before moving to the next sequential step.

### WorkflowContext

Every workflow runs with a shared context — a thread-safe key-value store that persists across steps and checkpoints. The context:

- **Automatically stores step outputs**: After step `fetch-logs` completes, its output is stored as `fetch-logs.output`.
- **Threads through templates**: Any subsequent step's `input`, `prompt`, or `vars` values can reference earlier outputs using `{{ctx.<key>}}`.
- **Supports custom keys**: Your prompts or step logic can write arbitrary keys (e.g., `temp.cache` for intermediate results).
- **Enforces a 16 KB limit per key**: Values larger than 16 KB are truncated with a warning printed to stderr. The full output remains in the pipeline run store (`internal/store/`), but only the truncated version is available for `{{ctx.*}}` template expansion in later steps.

Example context after two steps:
```json
{
  "input": "config=prod",
  "fetch-logs.output": "ERROR 2026-04-02 database timeout...",
  "analyze.output": "Root cause: connection pool exhausted",
  "temp.retry-count": "1"
}
```

### Decision Nodes

A `decision` step sends a prompt to your local Ollama instance and expects a JSON response. The model must return an object with a required `branch` field; the value determines which step executes next.

```yaml
- id: choose-action
  type: decision
  model: mistral
  prompt: |
    Error logs: {{ctx.logs.output}}
    
    Is this critical? Answer with:
    {"branch": "critical"} or {"branch": "low"}
  on:
    critical: notify-oncall
    low: log-and-continue
  default_branch: log-and-continue
  timeout_secs: 30
```

The conductor calls `POST http://localhost:11434/api/generate` (or your configured `--ollama-url`) with:
- `model`: the name you specified
- `prompt`: your expanded template
- `format: json`
- `stream: false`

The response is parsed for `{"branch": "<name>"}`. Execution branches to the step ID listed in `on.<name>`. If:
- Ollama times out (after `timeout_secs`, default 30)
- Returns invalid JSON or missing the `branch` field
- The returned branch name is not in the `on` map

Then the workflow branches to `default_branch`. If no default is set and any of these errors occur, the entire workflow fails.

### Parallel Branches

A `parallel` step contains multiple branches, each with its own sequence of steps. All branches execute concurrently and share the same `WorkflowContext`.

```yaml
- id: concurrent-checks
  type: parallel
  branches:
    - steps:
        - id: perf-check
          type: pipeline-ref
          pipeline: benchmark
          input: "data: {{ctx.input}}"
    - steps:
        - id: security-audit
          type: agent-ref
          agent: security-scanner
          input: "target: {{ctx.input}}"
```

Each branch:
- Reads all keys available in the shared context (outputs from earlier sequential steps, initial input, etc.)
- Stores its outputs back to the context (e.g., `perf-check.output`, `security-audit.output`)
- Runs independently and concurrently with other branches

The conductor waits for all branches to complete (success or failure). If any branch fails, the entire workflow fails. On success, outputs from all branches are available to subsequent sequential steps.

### Checkpointing and Resumability

After every step completes (success or failure), the conductor writes a checkpoint to the SQLite `workflow_checkpoints` table:

- Workflow name and run ID
- Step ID and execution status
- Step output (full, not truncated)
- Serialized `WorkflowContext` (JSON)
- Timestamp

If a step fails, the workflow halts immediately. Later, you resume with `glitch workflow resume --run-id <id>`. The runner:

1. Loads the workflow definition
2. Restores `WorkflowContext` from the last successful checkpoint
3. Finds the failed step
4. Re-executes that step
5. Continues from there (earlier steps are skipped)

This enables long-running workflows to survive transient failures and be resumed without losing context or replaying expensive setup steps.

### When to Use Workflows vs Pipelines

**Use a pipeline** if:
- You have a single, linear sequence of operations (fetch → analyze → report)
- All logic is self-contained and doesn't need to branch at runtime
- You don't need to compose multiple independent operations

**Use a workflow** if:
- You need to chain multiple pipelines or agents together
- Execution must branch based on runtime decisions (model output, file contents, etc.)
- You want automatic checkpointing so transient failures don't lose progress
- Steps need to share data (one step's output feeds into the next)
- You need parallel branches that run concurrently then rejoin

In short: pipelines are for individual tasks; workflows orchestrate tasks into larger processes.


## Configuration

### Workflow File Location

Workflow files live in `~/.config/glitch/workflows/` as `.workflow.yaml` files:

```
~/.config/glitch/workflows/
├── backup-and-verify.workflow.yaml
├── error-triage.workflow.yaml
├── morning-checklist.workflow.yaml
└── deploy-with-approval.workflow.yaml
```

The filename (minus the `.workflow.yaml` suffix) is the workflow name used in `glitch workflow run <name>`.

### Workflow YAML Schema

```yaml
name: <string, required>
  # Unique workflow identifier. Used in CLI commands and logging.
  # Examples: "backup-and-verify", "error-triage", "deploy-prod"

version: <string, required>
  # Semantic version for the workflow (e.g., "1.0", "1.1-beta").
  # Informational; not enforced by the runner.

steps: <array of WorkflowStep, required>
  # Ordered list of steps. Executed sequentially unless branching or parallelism applies.
```

### WorkflowStep Fields

```yaml
id: <string, required>
  # Unique identifier within the workflow.
  # Used in: templates ({{ctx.<id>.output}}), decision branching targets, checkpoints.
  # Must not contain spaces or special characters (letters, numbers, hyphens, underscores).

type: <string, required>
  # One of: "pipeline-ref", "agent-ref", "decision", "parallel"

# ===== For pipeline-ref steps =====
pipeline: <string, required if type: pipeline-ref>
  # Pipeline filename without `.pipeline.yaml` suffix.
  # Resolved to: ~/.config/glitch/pipelines/<pipeline>.pipeline.yaml
  # Example: "fetch-logs" → ~/.config/glitch/pipelines/fetch-logs.pipeline.yaml

input: <string, optional>
  # Input string passed to the pipeline.
  # Supports {{ctx.<key>}} template expansion for context values.
  # Example: input: "logs: {{ctx.fetch-logs.output}}\nconfig: {{ctx.config.output}}"

vars: <map[string]string, optional>
  # Key-value pairs merged into the pipeline's variable context.
  # Accessible in pipeline steps as {{.param.<key>}}.
  # Overrides defaults from the pipeline YAML.
  # Example:
  #   vars:
  #     timeout: "60"
  #     environment: "prod"

# ===== For agent-ref steps =====
agent: <string, required if type: agent-ref>
  # APM agent name (without "apm." prefix or ".pipeline.yaml" suffix).
  # Resolved to: ~/.config/glitch/pipelines/apm.<agent>.pipeline.yaml
  # Requires the agent to be installed via APM.
  # Example: "code-generator" → apm.code-generator.pipeline.yaml

# ===== For decision steps =====
model: <string, required if type: decision>
  # Ollama model name (e.g., "mistral", "llama2", "neural-chat").
  # Must be available in your local Ollama instance.

prompt: <string, required if type: decision>
  # Prompt sent to Ollama. Supports {{ctx.<key>}} template expansion.
  # Must produce JSON with a required "branch" field.
  # Example: "Classify: {{ctx.error.output}}. Return {\"branch\": \"critical\" or \"low\"}"

on: <map[string]string, required if type: decision>
  # Maps branch names (from the model's JSON "branch" field) to next step IDs.
  # If the model returns {"branch": "yes"}, execution routes to the step with id: "yes".
  # Example:
  #   on:
  #     critical: notify-oncall
  #     low: log-and-continue

default_branch: <string, optional>
  # Fallback step ID if Ollama request times out, returns invalid JSON, or the branch is not in "on".
  # If omitted and any of these errors occur, the entire workflow fails.

timeout_secs: <int, optional, default: 30>
  # Maximum seconds to wait for Ollama response.

# ===== For parallel steps =====
branches: <array of ParallelBranch, required if type: parallel>
  # Each branch is an independent sequence of steps, executed concurrently.
  # All branches share the same WorkflowContext.
  # Example:
  #   branches:
  #     - steps:
  #         - id: perf-check
  #           type: pipeline-ref
  #           pipeline: benchmark
  #     - steps:
  #         - id: cost-check
  #           type: pipeline-ref
  #           pipeline: cost-analyzer
```

### ParallelBranch

```yaml
- steps: <array of WorkflowStep, required>
    # List of steps in this branch.
    # Each step can be any type (pipeline-ref, agent-ref, decision, even nested parallel).
    # Branches run concurrently; the conductor waits for all to complete.
    # Outputs from each branch are stored as usual (<step_id>.output).
```

### CLI Commands

#### glitch workflow run

```bash
glitch workflow run backup-and-verify
glitch workflow run morning-prep --input "day=Monday"
glitch workflow run error-analysis --input "path=/var/log/app.log"
```

Starts a new workflow run, creating a record in `workflow_runs` and publishing `workflow.run.started` to BUSD.

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(none)* | Initial input string passed to the workflow; available in all steps via `{{ctx.input}}`. |
| `--ollama-url` | `http://localhost:11434` | Override the Ollama endpoint for decision nodes. |

#### glitch workflow resume

```bash
glitch workflow resume --run-id 42
glitch workflow resume --run-id 7
```

Resumes a workflow from the last successful checkpoint. The conductor loads the workflow definition, restores `WorkflowContext`, re-executes the failed step, and continues. Earlier completed steps are skipped.

| Flag | Required | Description |
|------|----------|-------------|
| `--run-id` | yes | Workflow run ID from a prior failed execution. |


## Examples

Save these in `~/.config/glitch/workflows/` and run with `glitch workflow run <name>`.

### Simple Pipeline Chain

File: `backup-and-verify.workflow.yaml`

```yaml
name: backup-and-verify
version: "1.0"
steps:
  - id: backup
    type: pipeline-ref
    pipeline: backup-database
    input: "target=prod"
  
  - id: verify
    type: pipeline-ref
    pipeline: verify-backup
    input: "backup-file: {{ctx.backup.output}}"
```

Run with:
```bash
glitch workflow run backup-and-verify
```

Execution:
1. Step `backup` runs the `backup-database` pipeline; output stored as `backup.output`
2. Step `verify` reads the backup file path from context and validates it
3. If either step fails, the workflow halts and can be resumed from the checkpoint

### Decision-Based Branching

File: `error-response.workflow.yaml`

```yaml
name: error-response
version: "1.0"
steps:
  - id: fetch-error
    type: pipeline-ref
    pipeline: get-latest-error
  
  - id: classify
    type: decision
    model: mistral
    prompt: |
      Error log:
      {{ctx.fetch-error.output}}
      
      Is this a critical production issue? Answer with:
      {"branch": "critical"} or {"branch": "low"}
    on:
      critical: page-oncall
      low: log-only
    default_branch: log-only
    timeout_secs: 20
  
  - id: page-oncall
    type: agent-ref
    agent: pagerduty-notifier
    input: "error: {{ctx.fetch-error.output}}"
  
  - id: log-only
    type: pipeline-ref
    pipeline: log-error
    input: "msg: {{ctx.fetch-error.output}}"
```

Execution flow:
1. Step `fetch-error` retrieves the latest error from the log
2. Step `classify` sends the error to Ollama (mistral model) for evaluation
3. If Ollama returns `{"branch": "critical"}`, execution jumps to `page-oncall`; otherwise to `log-only`
4. If Ollama times out (> 20 sec) or returns malformed JSON, uses `default_branch: log-only`
5. The appropriate notification step runs; earlier steps are not re-executed

### Parallel Branches

File: `concurrent-analysis.workflow.yaml`

```yaml
name: concurrent-analysis
version: "1.0"
steps:
  - id: gather-metrics
    type: pipeline-ref
    pipeline: collect-prometheus-metrics
    input: "timerange=1h"
  
  - id: parallel-checks
    type: parallel
    branches:
      - steps:
          - id: perf-analysis
            type: pipeline-ref
            pipeline: analyze-performance
            input: "metrics: {{ctx.gather-metrics.output}}"
      
      - steps:
          - id: cost-analysis
            type: pipeline-ref
            pipeline: analyze-cloud-costs
            input: "metrics: {{ctx.gather-metrics.output}}"
      
      - steps:
          - id: alert-check
            type: agent-ref
            agent: alert-analyzer
            input: "metrics: {{ctx.gather-metrics.output}}"
  
  - id: summarize
    type: pipeline-ref
    pipeline: build-report
    input: |
      perf: {{ctx.perf-analysis.output}}
      cost: {{ctx.cost-analysis.output}}
      alerts: {{ctx.alert-check.output}}
```

Execution flow:
1. Step `gather-metrics` runs sequentially and collects Prometheus data
2. Step `parallel-checks`: all three branches run simultaneously
   - `perf-analysis`: benchmarks the system
   - `cost-analysis`: evaluates cloud spending
   - `alert-check`: reviews active alerts
3. The conductor waits for all three branches to complete
4. Step `summarize` combines all outputs into a single report

All three branches can see the metrics from step 1 via `{{ctx.gather-metrics.output}}`, and their outputs are available to the final step.

### Using Variables and Inputs

File: `deploy-with-config.workflow.yaml`

```yaml
name: deploy-with-config
version: "1.0"
steps:
  - id: validate-env
    type: pipeline-ref
    pipeline: validate-deployment-env
    vars:
      environment: "staging"
      region: "us-west-2"
    input: "config-file: {{ctx.input}}"
  
  - id: build-image
    type: agent-ref
    agent: docker-builder
    vars:
      registry: "docker.example.com"
      tag: "v1.0.0"
    input: |
      validation-status: {{ctx.validate-env.output}}
      config: {{ctx.input}}
  
  - id: deploy
    type: pipeline-ref
    pipeline: deploy-to-k8s
    vars:
      namespace: "staging"
      strategy: "rolling"
    input: |
      image-sha: {{ctx.build-image.output}}
      environment: staging
```

Run with:
```bash
glitch workflow run deploy-with-config --input "config=staging.yaml"
```

The `--input` is stored as `ctx.input` and available to all steps. The `vars` in each step are passed to the pipeline as `{{.param.<key>}}` template parameters (see the [Pipelines](./pipelines.md) documentation for details).

### Resumable Multi-Step Workflow

If the `deploy` step fails in the previous example:

```bash
# First run — fails at "deploy" step
$ glitch workflow run deploy-with-config --input "config=staging.yaml"
# Error output includes: run_id: 7

# Later: resume from the checkpoint
$ glitch workflow resume --run-id 7

# The conductor:
# 1. Loads the workflow definition
# 2. Restores WorkflowContext (input, outputs from validate-env and build-image, etc.)
# 3. Re-executes the "deploy" step (with fresh state)
# 4. Skips "validate-env" and "build-image" (already completed)
```

This saves time and avoids re-running expensive validation and image-build steps.

### Complex Workflow with Decisions and Parallel Branches

File: `end-to-end-testing.workflow.yaml`

```yaml
name: end-to-end-testing
version: "1.0"
steps:
  - id: run-tests
    type: pipeline-ref
    pipeline: pytest-suite
    input: "environment=staging"
  
  - id: analyze-results
    type: decision
    model: mistral
    prompt: |
      Test results:
      {{ctx.run-tests.output}}
      
      Should we proceed with deployment?
      Return {"branch": "proceed"} or {"branch": "investigate"}
    on:
      proceed: run-smoke-tests
      investigate: notify-team
    default_branch: notify-team
    timeout_secs: 15
  
  - id: run-smoke-tests
    type: parallel
    branches:
      - steps:
          - id: smoke-api
            type: pipeline-ref
            pipeline: smoke-test-api
            input: "endpoint: https://staging.example.com"
      
      - steps:
          - id: smoke-ui
            type: agent-ref
            agent: browser-tester
            input: "url: https://staging.example.com"
  
  - id: notify-team
    type: pipeline-ref
    pipeline: send-slack-notification
    input: |
      status: "Tests passed, smoke tests completed"
      api-results: {{ctx.smoke-api.output}}
      ui-results: {{ctx.smoke-ui.output}}
```

Execution flow:
1. Run full test suite
2. Ask Ollama whether to proceed (branching decision)
3. If proceeding, run API and UI smoke tests in parallel
4. Notify the team with combined results
5. If analysis suggested investigating, skip smoke tests and notify immediately


## See Also

- [Pipelines](/docs/pipelines.md) — pipeline YAML syntax and step types within workflows
- [Architecture](/docs/architecture.md) — gl1tch system design and component overview
- [Console](/docs/console.md) — TUI reference and workflow management from the dashboard
- [Philosophy](/docs/philosophy.md) — why pipelines and workflows are declarative and auditable
