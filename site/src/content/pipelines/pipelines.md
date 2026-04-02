## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

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

I have enough information to write comprehensive pipeline documentation based on the OpenSpec proposals, code examples, and architectural details provided.

```markdown
---
title: Pipelines
description: Define and execute multi-step workflows as DAGs with structured data flow, parallel execution, and error handling.
order: 20
---

Pipelines are declarative YAML workflows that orchestrate a sequence of computational steps. Each step is an executor — a model, plugin, or built-in function — and steps can run in parallel, pass structured data to downstream steps, retry on failure, and branch based on conditions. Pipelines are the primary way to automate tasks in glitch.

## Concepts

A pipeline is a directed acyclic graph (DAG) where vertices are steps and edges represent data dependencies. You define it once as a `.pipeline.yaml` file, then execute it with `glitch pipeline run <name>` or via the UI. The runner:

1. Parses the YAML and builds the dependency graph.
2. Executes independent steps concurrently using goroutines.
3. Waits for all dependencies of a step to finish before starting it.
4. Passes output from earlier steps as typed context data to later steps.
5. Publishes events to the event bus at key lifecycle points.

Steps are not scripts — they are structured executor calls. A step invokes a model, a shell plugin, a built-in operation, or an APM agent. The step definition includes the executor ID, parameters, retry policy, and what to do if it fails.


## When to Use Pipelines vs Workflows

**Use a pipeline** for a single, isolated computation: run a model on code, fetch and process data with jq, generate a daily digest. Pipelines are fast to author, easy to debug, and work offline with local LLMs.

**Use a workflow** when you need to sequence multiple pipelines or agents, branch based on LLM decisions, or coordinate across multiple execution contexts. Workflows sit above pipelines and let you compose them with checkpointing, resumable execution, and centralized state. You will author workflows separately once they are stable; for now, focus on pipelines.


## Pipeline Structure

A minimal pipeline has a name and at least one step:

```yaml
name: my-pipeline
version: "1"
steps:
  - id: analyze
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Analyze this code for security issues:
      {{input}}
```

Every pipeline has a `name` (used for discovery and running), a `version` (always "1" for now), and a `steps` array. Each step must have a unique `id` within the pipeline.


## Step Fields

Every step declares:

- **id**: Unique identifier for this step within the pipeline.
- **executor**: Which executor to invoke — `claude`, `ollama`, a plugin name like `jq`, or a built-in like `builtin.assert`.
- **model**: (if executor is `claude` or `ollama`) The specific model to use — `claude-sonnet-4-6`, `mistral`, `llama3.2`, etc.
- **prompt**: (if the executor accepts text input) The template string to pass. Can reference `{{input}}` or other step outputs.

Optional fields:

- **needs**: List of step IDs this step depends on. If omitted, steps run in order.
- **input**: Input data for this step, as a string or template. For model steps, this becomes part of the prompt context.
- **params**: Key-value parameters passed to the executor (executor-specific).
- **retry**: Retry policy — `max_attempts`, `interval`, optional `on` condition.
- **on_failure**: Name of a recovery step to run if this step fails.
- **publish_to**: Event bus topic to publish the step's output to (for monitoring and downstream listeners).


## Dependencies and Parallel Execution

By default, steps run in the order they appear. To run steps in parallel or express dependencies, use the `needs` field:

```yaml
steps:
  - id: fetch_data
    executor: jq
    params:
      filter: ".items[] | {id, title}"

  - id: enrich_with_claude
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Enrich this data with descriptions:
      {{step.fetch_data.output}}
    needs: [fetch_data]

  - id: summarize
    executor: ollama
    model: mistral
    prompt: |
      Summarize these enriched items:
      {{step.enrich_with_claude.output}}
    needs: [enrich_with_claude]
```

The runner builds a DAG from the `needs` declarations and executes independent steps in parallel. If a step has no `needs`, it can start immediately. Steps wait for all their dependencies to finish before starting.


## Context and Data Flow

Output from each step is stored in a typed context map and available to downstream steps using the syntax `{{step.<step_id>.<field>}}`. For steps that produce JSON or structured data, you can drill down: `{{step.fetch.output.items}}`.

The context is passed through the entire pipeline execution, so later steps see the full history. This is how you chain operations: one step's output becomes the next step's input.

```yaml
steps:
  - id: list_files
    executor: builtin.shell
    params:
      command: find . -name "*.go" -type f

  - id: analyze_each
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      For each file, suggest optimizations:
      {{step.list_files.output}}
    needs: [list_files]
```


## Step Lifecycle

Each step goes through three phases:

1. **init**: Allocate resources, validate inputs. If init fails, the step does not run.
2. **execute**: Perform the work — call the model, run the plugin, etc.
3. **cleanup**: Release resources. Always runs, even if execute fails.

The runner handles these transparently; you only need to know that cleanup always happens. This ensures files are closed, connections are released, and temporary data is cleaned up.


## Retry and Error Handling

Use the `retry` field to automatically retry a failed step:

```yaml
steps:
  - id: fetch_from_api
    executor: builtin.http_get
    params:
      url: https://api.example.com/data
    retry:
      max_attempts: 3
      interval: 5s
      on: on_failure
```

If a step fails and has no retry, or retries exhaust, use `on_failure` to specify a recovery step:

```yaml
steps:
  - id: fetch
    executor: builtin.http_get
    params:
      url: https://api.example.com/data
    on_failure: use_cache

  - id: use_cache
    executor: builtin.log
    params:
      message: "API down, using cached data"
```

If a step fails and has no recovery, the entire pipeline stops and reports the error.


## Built-in Executors

gl1tch includes a small set of built-in steps that don't require a plugin:

- **builtin.assert**: Fail the pipeline if a condition is false.
- **builtin.set_data**: Store a value in the context for later steps.
- **builtin.log**: Write a message to the log.
- **builtin.sleep**: Pause execution for a duration.
- **builtin.http_get**: Fetch data from a URL.
- **builtin.shell**: Execute a shell command.

These are useful for simple data manipulation, debugging, and orchestration without external plugins.


## Plugins and Executors

Beyond built-in steps, you can use any installed plugin. Common ones:

- **jq**: Filter and transform JSON. Use in a pipeline step via `executor: jq` and pass your filter as a param.
- **gh**: GitHub CLI. Requires a `gh` plugin sidecar descriptor in `orcai-plugins/plugins/gh/gh.yaml`.
- **opencode**: Run code-aware LLM tasks locally via Ollama.
- **claude**: Call Claude Haiku, Sonnet, or Opus. Requires a configured provider.
- **ollama**: Run a local model via Ollama. Requires Ollama to be running.

Plugin parameters are executor-specific. Check the plugin documentation or sidecar descriptor for details.


## Real Example: GitHub Activity Digest

Here is a complete pipeline that fetches GitHub issues, enriches them with a local LLM, and formats the output as JSON:

```yaml
name: github-activity-digest
version: "1"
steps:
  - id: fetch_issues
    executor: gh
    params:
      args: "issue list --json title,url,state --limit 10"

  - id: normalize
    executor: jq
    params:
      filter: ".[] | {title, url, status: .state}"

  - id: enrich
    executor: opencode
    model: qwen2.5
    prompt: |
      For each issue, add a 1-sentence summary:
      {{step.normalize.output}}

  - id: final_json
    executor: claude
    model: claude-haiku
    prompt: |
      Format this output as a JSON object with keys "digest" (array of enriched issues) and "timestamp":
      {{step.enrich.output}}

  - id: validate
    executor: jq
    params:
      filter: ".digest | length > 0"
    on_failure: report_empty

  - id: report_empty
    executor: builtin.log
    params:
      message: "No issues found"
```

Run this with `glitch pipeline run github-activity-digest`. The runner fetches issues, processes them through enrichment and formatting, validates the JSON, and logs the result. If validation fails, it logs a message instead of stopping.


## Writing Pipelines from the CLI

Use `/pipeline` in glitch to create a new pipeline interactively:

```
/pipeline my-new-task
```

The UI will ask you to describe what the pipeline should do, then generate the YAML for you. You can edit it further in the pipeline editor and save it.


## Observability

The runner publishes BUSD events at key points:

- `pipeline.started`: Pipeline execution begins.
- `pipeline.step.started`: A step is about to run.
- `pipeline.step.done`: A step completed successfully.
- `pipeline.step.failed`: A step failed.
- `pipeline.finished`: Pipeline execution ended.

Monitors and other tools can subscribe to these topics to track progress, log results, or trigger downstream actions.


## Best Practices

Keep pipelines single-purpose. If you have multiple distinct tasks, split them into separate pipelines and compose them with a workflow later. This makes them easier to test, reuse, and reason about.

Use meaningful step IDs so context references are clear. `fetch_github_issues` is better than `step_1`.

Always specify `needs` explicitly if you need parallel execution. Relying on step order is fragile and hard to debug.

For long-running or flaky operations, always use `retry`. Set `max_attempts` to at least 2 and `interval` to a reasonable backoff.

Use built-in steps for simple operations like logging or sleeping. They're faster and have no external dependencies.

Test pipelines with small inputs first. Run locally with `ollama` or a cached test dataset before hitting external APIs.


## See Also

- [Executors and Providers](/docs/executors) — detailed plugin and model configuration
- [Workflows](/docs/workflows) — compose multiple pipelines with branching and checkpointing
- [Brain Context](/docs/brain) — inject past learnings into pipeline steps
- [Command Reference](/docs/commands) — `glitch pipeline` CLI
```

