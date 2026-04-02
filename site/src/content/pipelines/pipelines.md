## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[write_doc] ` XML blocks. After the step completes, the runner parses the response for any blocks matching this structure:

```xml
<brain type="research" tags="optional,comma,tags" title="Human readable title">
Your insight, analysis, or structured data here.
[deep_search] blocks that get embedded as vectors and stored per-cwd in sqlite. injected automatically as context on future runs. ^spc b to browse.
- cron: pipelines can run on a schedule — daily digests, nightly reviews, morning prep.
- events: you get notified when pipelines finish or fail. you can analyze the results and suggest what to do next.
- git worktrees: glitch uses git worktrees for isolated pipeline runs. if the user's cwd is a worktree (check: git worktree list, or .git is a file not a dir), remind them to merge or clean it up if the work looks done. don't nag — mention it once when it's relevant, like after a pipeline finishes or when they ask about next steps.

help the user build pipelines, understand their codebase, automate tasks, debug runs, manage brain notes.
keep answers short — a few sentences unless more is clearly needed.
no markdown headers, no bullet lists. write in sentences.
don't narrate your own personality. just be it.`

type glitchBackend interface {
	streamIntro(ctx context.Context, cwd string) (<-chan string, error)
	// brainCtx is injected as an extra system message before the conversation.
	// Pass "" to skip brain injection (e.g. run-analysis already embeds it).
	

=== file:/Users/stokes/Projects/gl1tch/openspec/changes/pipeline-brain-context/design.md:L1-L11 ===
## Context

ORCAI pipelines dispatch steps to AI agent plugins (e.g. `claude.chat`, `ollama.chat`). Each plugin receives a `prompt` string and a flat `vars` map. Currently there is no mechanism for the pipeline runner to automatically enrich that prompt with knowledge from the ORCAI SQLite store — agents are blind to historical runs, stored notes, or any other persistent data unless a pipeline author manually writes `db` steps and threads the results through template variables. The `db` step is awkward, requires SQL literacy, pollutes the pipeline YAML with boilerplate, and provides no guardrails against accidental writes to operational tables.

The target state: pipeline YAML gains two optional boolean flags — `use_brain` (read) and `write_brain` (write) — that activate automatic, transparent pre-context injection around agent steps. A new `BrainInjector` component in `internal/pipeline` assembles the context string, and a new `brain_notes` table isolates agent-written knowledge from the operational `runs` table.

## Goals / Non-Goals

**Goals:**
- `use_brain: true` on a pipeline or step causes the runner to prepend a read-context block to the agent's prompt, describing the ORCAI schema and supplying recent relevant data from the DB
- `write_brain: true` on a pipeline or step causes the runner to (a) include a write-context block instructing the agent to embed `<brain>` XML in its response, and (b) parse and persist that XML to `brain_notes` after the step completes
- `brain

=== file:/Users/stokes/Projects/gl1tch/openspec/changes/pipeline-brain-context/specs/pipeline-brain-feedback/spec.md:L1-L18 ===
## ADDED Requirements

### Requirement: Brain notes written in a pipeline run are surfaced to subsequent use_brain steps in the same run
When a pipeline has both `use_brain` and `write_brain` active (at any combination of pipeline or step level), the `BrainInjector.ReadContext` implementation SHALL include any `brain_notes` rows already written during the current pipeline run in the preamble delivered to later steps. Notes are ordered newest-first. This creates a within-run feedback loop: each agent step can build on insights recorded by earlier steps.

#### Scenario: Note written by step A appears in preamble of step B
- **WHEN** step A (with `write_brain: true`) runs first and inserts a brain note, and step B (with `use_brain: true`) runs after
- **THEN** step B's prompt preamble includes the body of the note written by step A

#### Scenario: Note from a different run is not included
- **WHEN** a brain note with a different `run_id` exists in the store
- **THEN** that note does NOT appear in the preamble of any step in the current run

#### Sc...[truncated]

> Do NOT modify the runs table.

---
BRAIN NOTE INSTRUCTION: Include a <brain> block somewhere in your response to persist an insight for future steps in this pipeline.

Use the <brain> tag with structured attributes to categorize your note:

  <brain type="research" tags="optional,comma,tags" title="Human readable title">
  Your insight, analysis, or structured data here.
  </brain>

Available types:
- research  — background info, context, references
- finding   — concrete discovery (bug, pattern, fact)
- data      — structured output (metrics, counts, lists)
- code      — code snippet or file path reference

The <tags> attribute is optional. The <title> attribute is recommended.

Example:
  <brain type="finding" tags="auth,security" title="Session token stored in plain text">
  Found that session tokens are written to ~/.glitch/session without encryption.
  File: internal/auth/session.go line 42.
  </brain>

The brain note will be stored and made available to subsequent agent steps with use_brain enabled.
---

---
title: "Pipelines"
description: "DAG execution, step lifecycle, context passing, brain injection, and failure handling in gl1tch pipelines."
order: 20
---

Pipelines are YAML-defined multi-step workflows where each step dispatches to a provider (local Ollama model, Claude, or builtin executor) with a prompt and optional configuration. Steps form a directed acyclic graph (DAG) and execute in parallel when their dependencies are satisfied. Output flows forward through template variables, brain notes inject persistent context, and failure is handled through retries and fallback routing.


## DAG Execution Model

Pipelines execute as a dependency-driven graph. Each step declares an optional `needs` list naming the steps that must complete before it can start. The runner builds a DAG from these declarations and executes independent steps concurrently using goroutines.

Steps not listed in any `needs` array execute immediately when the pipeline starts. Once all dependencies of a step are complete, it enters the execution queue. Steps with no dependencies start in parallel, reducing total pipeline runtime.

The runner maintains step state throughout execution. A step cannot start until all its `needs` have reached a terminal state (completed successfully, failed, or exhausted retries). If a dependency fails and has no `on_failure` handler, the dependent step is cancelled; if the dependency has an `on_failure` handler, the dependent waits for the recovery step to complete before proceeding.


## Step Lifecycle

Each step progresses through three phases: **init**, **execute**, and **cleanup**. Plugins can allocate resources during init (e.g., establish a database connection), use those resources during execute (the main prompt/response loop), and release them cleanly in cleanup. Builtin executors (assert, log, set_data) run only the execute phase.

The init phase is optional. If a plugin defines it, the runner calls it before execute. If init fails, the step fails immediately without attempting execute or cleanup. Execute is mandatory — it contains the actual work (prompt dispatch, response parsing, etc.). Cleanup is optional and runs regardless of execute success or failure, ensuring resources are always released.

Output from execute is captured and made available to subsequent steps through template variables. Cleanup output is not exposed to the context; its purpose is resource management only.


## Context and Variable Passing

Step output flows forward through template variables. A step with ID `analyze` exposes its output in two forms:

- `{{.steps.analyze.output}}` — the raw text output (for backwards compatibility)
- `{{.step.analyze.data.*}}` — structured output fields (JSON parsed from the response)

When a step's provider returns JSON, the runner attempts to parse the top-level keys into the `step.<id>.data` namespace. For example, if a step returns `{"bugs": [...], "summary": "..."}`, you can reference `{{.step.analyze.data.bugs}}` and `{{.step.analyze.data.summary}}` in subsequent steps.

Pipeline-level and step-level variables are also accessible:

- `{{cwd}}` — the working directory where the pipeline is executing
- `{{param.<name>}}` — variables passed to the pipeline runner (e.g., `{{param.input}}` for user-provided input)

Template expansion happens at step execution time. The runner substitutes all `{{...}}` tokens in the step's prompt and input fields before dispatching to the executor.


## Brain Injection

Pipelines can tap into gl1tch's persistent brain system through two flags: `use_brain` to read stored notes and `write_brain` to persist new insights.

When `use_brain: true` on a pipeline or step, the runner prepends a read-context block to the agent's prompt before execution. This block includes the ORCAI schema (a summary of the database structure) and recent brain notes relevant to the current working directory. Brain notes are ranked by recency and semantic similarity; only the most relevant ones are injected to avoid overwhelming the prompt.

When `write_brain: true`, the runner includes instruction text in the prompt asking the agent to embed insights in `<brain>` XML blocks. After the step completes, the runner parses the response for any blocks matching this structure:

```xml
<brain type="research" tags="optional,comma,tags" title="Human readable title">
Your insight, analysis, or structured data here.
</brain>
```

Available types are `research` (background info, references), `finding` (concrete discovery), `data` (structured output like metrics), and `code` (code snippets or file paths). The tags and title are optional but recommended. The runner extracts each block, embeds it as a vector, and stores it in the `brain_notes` table scoped to the current working directory.

Within a single pipeline run, brain notes written by earlier steps are available to later steps. A step with `use_brain: true` that runs after a step with `write_brain: true` will receive those newly-written notes in its prompt preamble, creating a feedback loop where agents build on each other's insights.


## Retry and Failure Handling

Steps can declare a `retry` block to tolerate transient failures. The retry block specifies:

- `max_attempts` — total number of tries (1 means no retry, just one attempt)
- `interval` — duration to wait between retries (e.g., `5s`, `1m`)
- `on` — optional condition (`always` or `on_failure`; default is `on_failure`)

When a step fails and has retries remaining, the runner waits for the interval, then re-executes the step. If all retries are exhausted, the step fails permanently.

For unrecoverable failures, steps can declare an `on_failure` handler naming another step to run instead. If a step fails (after all retries), the runner cancels its dependents and immediately enqueues the failure handler. The failure handler can assess the error, log it, or attempt recovery. If the failure handler succeeds, dependents of the original step are re-queued; if it fails, the entire pipeline fails.

Retries apply only to transient errors (network timeouts, rate limits) that the executor deems retryable. Permanent errors (authentication failure, malformed input) fail immediately regardless of retry configuration.


## Builtin Executors

The runner includes a small set of built-in step types that don't require an external plugin:

- `builtin.assert` — evaluate a boolean condition; fail if false
- `builtin.set_data` — inject static data into the execution context (useful for seeding variables or feature flags)
- `builtin.log` — write a message to the pipeline run log
- `builtin.sleep` — pause for a specified duration
- `builtin.http_get` — fetch content from a URL and expose the response as step output

These are compiled into the gl1tch binary and execute synchronously in the step execution loop.


## Real Examples


### Sequential Steps with Context Passing

```yaml
name: code-review
version: "1"
steps:
  - id: fetch_diff
    executor: builtin.http_get
    input: "{{param.git_diff_url}}"

  - id: analyze
    executor: claude
    model: claude-sonnet-4-6
    use_brain: true
    prompt: |
      Review this code diff and list potential bugs:
      {{.steps.fetch_diff.output}}

  - id: summarize
    executor: ollama
    model: llama3.2
    write_brain: true
    needs:
      - analyze
    prompt: |
      Summarize this review in 3 bullet points:
      {{.steps.analyze.output}}
```

Step `fetch_diff` runs first (no dependencies). Step `analyze` waits for it and receives the diff output. Step `summarize` waits for `analyze`, writes a brain note, and references the analysis.


### Parallel Execution with Retry

```yaml
name: parallel-validation
version: "1"
steps:
  - id: lint
    executor: builtin.assert
    retry:
      max_attempts: 3
      interval: 2s
    prompt: "{{param.lint_output}}"

  - id: test
    executor: builtin.assert
    retry:
      max_attempts: 3
      interval: 2s
    prompt: "{{param.test_output}}"

  - id: publish
    executor: builtin.log
    needs:
      - lint
      - test
    prompt: "All checks passed."
```

`lint` and `test` run in parallel with automatic retry. `publish` waits for both, then logs success.


### Failure Handling

```yaml
name: resilient-pipeline
version: "1"
steps:
  - id: fetch_data
    executor: ollama
    model: mistral
    on_failure: log_error
    prompt: "Fetch data from API"

  - id: log_error
    executor: builtin.log
    prompt: "Failed to fetch data. Skipping this step."

  - id: process
    executor: ollama
    model: llama3.2
    needs:
      - fetch_data
    prompt: "Process the data"
```

If `fetch_data` fails, `log_error` runs instead. Dependents of `fetch_data` (`process`) are cancelled.


### Brain Feedback Loop

```yaml
name: iterative-refinement
version: "1"
steps:
  - id: draft
    executor: claude
    model: claude-haiku-4-5
    write_brain: true
    prompt: |
      Draft a brief proposal for {{param.feature}}.
      <brain type="research" title="Feature proposal">...</brain>

  - id: refine
    executor: claude
    model: claude-sonnet-4-6
    use_brain: true
    needs:
      - draft
    prompt: |
      Refine this proposal, building on prior insights:
      {{.steps.draft.output}}
```

`refine` receives the brain note written by `draft` in its prompt preamble, enabling informed iteration.


## See Also

- [Brain System](/docs/brain) — how to write and query brain notes
- [Configuration](/docs/configuration) — setting up providers and model credentials
- [CLI Reference](/docs/cli) — `glitch pipeline` command options

