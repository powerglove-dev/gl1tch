---
title: "Pipelines"
description: "Declarative multi-step workflows as a DAG with dependency injection, parallel execution, and structured output context."
order: 2
---

Pipelines are declarative workflows defined in YAML that gl1tch executes as a directed acyclic graph (DAG). Each step runs an executor—shell command, LLM model, builtin function, or plugin—and its output becomes context for downstream steps. Pipelines are auditable, repeatable, and chainable: once a pipeline finishes, its outputs feed the brain so the next pipeline runs smarter.


## Architecture Overview

A pipeline lives in `~/.config/glitch/pipelines/<name>.pipeline.yaml` and is executed by the runner in `internal/pipeline/runner.go`. The runner constructs a DAG from step `Needs` declarations, executes independent steps concurrently with goroutines, and manages an `ExecutionContext` that threads data between steps via template expressions.

**Data flow:**
1. Runner loads pipeline YAML → validates DAG (cycle detection, unknown references)
2. Builds adjacency list mapping each step to its dependents
3. Starts all steps with no dependencies
4. As each step completes, enqueues its dependents if all their `Needs` are satisfied
5. Collects step outputs into a shared context map: `step.<id>.data.<key>`
6. Passes context to downstream steps via template interpolation
7. Persists final outputs and brain notes to the store

**Packages:**
- `internal/pipeline` — core DAG builder, runner, executor interface, and builtins
- `internal/executor` — registered executors (shell, LLM, plugins)
- `internal/store` — SQLite checkpoint of runs and brain notes
- `internal/brainrag` — optional RAG-based context injection

## Technologies

- **YAML** — declarative, version-controllable step definitions
- **Goroutines** — parallel step execution within max_parallel bounds
- **Go templates** — interpolation of `{ {steps.<id>.<key>}}` and builtin functions
- **SQLite** — persistent storage of run history and brain output


## Concepts

**Step**
A single unit of work: shell command, LLM prompt, user input, output write, or builtin action. Steps run within a lifecycle (Init → Execute → Cleanup) and can declare retry policies, failure handlers, and forwarding expressions.

**Executor**
A handler that runs a step: `shell`, `claude`, `ollama`, `builtin.assert`, `builtin.log`, etc. Executors are registered in `internal/executor` and identified by hierarchical name: `category.action`.

**DAG (Directed Acyclic Graph)**
The dependency graph formed by step `Needs` declarations. The runner detects cycles at load time and executes steps as soon as all their dependencies complete.

**Needs**
A list of step IDs that must complete before this step runs. Used to declare sequential or parallel chains. If `needs: []` (or omitted), the step runs immediately.

**Condition**
An expression evaluated after a step completes to decide whether to run the next step, skip it, or route to a failure handler. Syntax: `always`, `not_empty`, `contains:<string>`, `matches:<regex>`, `len > <n>`.

**ExecutionContext**
Shared mutable state (map[string]any) passed between steps. Accessed via dot-separated paths: `step.fetch.data.url`. Step outputs are stored as `step.<id>.data.<key>`.

**Template Expression**
Interpolation syntax in step Prompt, Input, Args, and Vars fields:
- `{ {steps.<id>.<key>}}` — access step output by ID and key
- `{{.param.<name>}}` — access pipeline-level var
- `{{env "VAR_NAME"}}` — read environment variable
- `{{"<expr>" | upper | trim}}` — builtin functions (replace, split, join, default, get, etc.)

**ForEach**
When a step declares `for_each: <list>`, the runner clones it N times (one per item) and collects outputs into an array. Items are newline-separated strings or a template expression that resolves to a list.

**Brain**
Persistent embedding store (`internal/store`) that checkpoints step outputs and LLM reasoning blocks (`<brain>` XML) from past runs. Pipelines can inject brain context into prompts so later steps see historical knowledge without manual threading.


## Configuration / YAML Reference

**Pipeline top-level:**
- `name` (required, string) — pipeline identifier; used in discovery and routing
- `version` (string) — semantic version; reserved for schema evolution
- `description` (string) — one-sentence summary for discovery and routing
- `trigger_phrases` (array of strings) — example imperative invocations ("run git pulse", "review my PR") embedded in the intent router instead of description when present
- `steps` (array of Step) — ordered step definitions
- `vars` (map) — pipeline-level context available to all steps as `{{.param.<name>}}`
- `max_parallel` (integer) — maximum concurrent steps; defaults to 8 when zero
- `write_brain` (boolean) — if true, all steps inject brain write instructions (can be overridden per step)
- `game` (tri-state boolean, pointer) — nil=enabled, false=disabled; gates game scoring for this pipeline

**Step fields:**

Core execution:
- `id` (required, string) — unique step identifier within the pipeline
- `executor` (string) — executor name: `shell`, `claude`, `ollama`, `builtin.assert`, `category.action`, etc. Defaults to LLM if omitted and model is set.
- `model` (string) — Ollama or Claude model name; required for LLM steps
- `type` (string) — `input` (user prompt), `output` (write to disk), or omitted (executor step)

Input & output:
- `prompt` (string) — LLM prompt or shell command body; supports template syntax
- `input` (string) — user-facing prompt for `type: input` steps
- `args` (map) — structured arguments passed to executor; supports nested objects and template expressions (supersedes `vars`)
- `vars` (map[string]string) — flat string variables passed to executor; used when `args` is not set
- `inputs` (map[string]string) — explicit mapping of input names to template expressions, resolved before execution
- `outputs` (map[string]string) — declares output keys that this step produces; full output string stored under each key
- `publish_to` (string) — event bus topic to forward this step's output (e.g., `orcai.pipeline.step.done`)

Branching & control flow:
- `needs` (array of strings) — list of step IDs that must complete before this step runs; empty or omitted means run immediately
- `condition` (string) — post-execution branch condition: `always`, `not_empty`, `contains:<str>`, `matches:<re>`, `len > <n>`; if false, skip downstream dependents
- `retry` (object) — retry policy with `max_attempts` (int), `interval` (duration string), `backoff` (bool); applied on executor error
- `on_failure` (string) — step ID to run if this step fails after all retries; exclusive with condition routing
- `for_each` (string) — newline-separated list or template expression resolving to array; clones step N times and collects outputs

Brain & context:
- `no_brain` (boolean) — if true, suppress brain context injection (use for output-generation steps where brain markup would leak into content)
- `write_brain` (tri-state boolean, pointer) — nil=inherit pipeline setting, true=force on, false=force off
- `no_clarify` (boolean) — if true, suppress GLITCH_CLARIFY instruction for automated steps
- `prompt_id` (string) — title of a saved prompt in the store; prepended to prompt body with blank-line separator (case-insensitive matching)

**Retry policy:**
```yaml
retry:
  max_attempts: 3
  interval: "5s"
  backoff: true     # exponential backoff (not implemented yet, reserved)
  on: "always"      # or "on_failure" (implicit default)
