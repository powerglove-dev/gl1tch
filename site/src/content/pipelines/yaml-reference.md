---
title: "Pipeline YAML Reference"
description: "Every field, what it does, and when to use it."
order: 2
---

## Pipeline-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | `string` | *required* | Unique name for the pipeline. Used in logs, the store, and as a lookup key. |
| `version` | `string` | `""` | Schema version. Currently always `"1"`. |
| `description` | `string` | `""` | Human-readable summary of what the pipeline does. Used by the intent router to match `glitch ask` prompts to the right pipeline. |
| `steps` | `Step[]` | *required* | Ordered list of steps. Execution order is determined by `needs` dependencies, not array position. |
| `vars` | `map[string]any` | `{}` | Pipeline-level seed context. Available to all steps via template expressions. |
| `max_parallel` | `int` | `8` | Maximum number of steps that can run concurrently. Only matters for DAG pipelines with parallel branches. |

## Step-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | `string` | *required* | Unique identifier for the step. Referenced by `needs`, `on_failure`, and template expressions. |
| `executor` | `string` | `""` | The executor to run. Native executors: `claude`, `ollama`. Builtins: `builtin.assert`, `builtin.log`, etc. Sidecar wrappers: `gh`, `jq`, `write`, or any name registered in `~/.config/glitch/wrappers/`. |
| `model` | `string` | `""` | Model identifier passed to the executor. For `claude`: e.g. `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`. For `ollama`: the local model name like `qwen2.5-coder:latest`. |
| `prompt` | `string` | `""` | The prompt text sent to AI executors. Supports `{{template}}` expressions for variable interpolation. |
| `input` | `string` | `""` | Raw input string. Used by non-AI executors or to override the default input. Supports template expressions. |
| `publish_to` | `string` | `""` | Topic name. When set, the step's output is published to the event bus under this topic. |
| `needs` | `string[]` | `[]` | Step IDs that must complete before this step runs. Creates the DAG execution order. |
| `vars` | `map[string]string` | `{}` | Flat key-value pairs passed to the executor as environment/template variables. For the `gh` executor, use `vars.args` to pass CLI arguments. |
| `args` | `map[string]any` | `{}` | Structured data passed to the executor. Supersedes `vars` when set. Used by builtins that need typed input. |
| `condition` | `Condition` | `{}` | Conditional branching. See Conditions below. |
| `for_each` | `string` | `""` | Template expression or newline-separated list. When set, the step is cloned once per item. The current item is available as `{{item}}`. |
| `retry` | `RetryPolicy` | `null` | Retry policy for this step. See Retry below. |
| `on_failure` | `string` | `""` | Step ID to run if this step fails after all retry attempts. |
| `prompt_id` | `string` | `""` | Title of a saved prompt in the store. When set, the prompt body is prepended to the step's input before execution. Case-insensitive title matching. |
| `outputs` | `map[string]string` | `{}` | Declares output keys produced by this step. After completion, the full output string is stored under each declared key. |
| `inputs` | `map[string]string` | `{}` | Maps input names to template expressions like `{{steps.<id>.<key>}}`. Resolved before execution using accumulated step outputs. |
| `write_brain` | `bool` | `false` | When true, the step's output is written to the brain store after execution. Later runs can inject this context via `--brain`. |

## Conditions

The `condition` field enables branching. It has three sub-fields:

| Field | Type | Description |
|-------|------|-------------|
| `if` | `string` | Expression to evaluate against the previous step's output. |
| `then` | `string` | Step ID to jump to if the expression is true. |
| `else` | `string` | Step ID to jump to if the expression is false. |

### Condition expressions

| Expression | Matches when |
|------------|-------------|
| `always` | Always true. |
| `not_empty` | Output is not empty after trimming whitespace. |
| `contains:<str>` | Output contains the literal string `<str>`. |
| `matches:<regex>` | Output matches the Go regex pattern `<regex>`. |
| `len > N` | Output length (in bytes) exceeds `N`. |

Example:

```yaml
- id: classify
  executor: claude
  model: claude-haiku-4-5-20251001
  prompt: "Is this Go or Python code? Reply with one word."
  condition:
    if: "contains:Go"
    then: go-handler
    else: python-handler
```

## Retry policy

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | `int` | `0` | Number of retry attempts. `0` means no retry. |
| `interval` | `duration` | `"0s"` | Time between retries. Supports Go duration strings: `"2s"`, `"500ms"`, `"1m"`. |
| `on` | `string` | `"always"` | When to retry: `"always"` retries on any completion, `"on_failure"` retries only on error. |

Example:

```yaml
- id: flaky-api
  executor: builtin.http_get
  args:
    url: "https://api.example.com/data"
  retry:
    max_attempts: 3
    interval: "2s"
    on: on_failure
```

## Template expressions

Templates use `{{double braces}}` and are resolved before step execution.

| Expression | Resolves to |
|------------|-------------|
| `{{steps.<id>.output}}` | The string output of a completed step. |
| `{{steps.<id>.<key>}}` | A specific output key declared in the step's `outputs` map. |
| `{{vars.<name>}}` | A pipeline-level variable from the `vars` map. |
| `{{item}}` | The current iteration value inside a `for_each` step. |

## Complete annotated example

```yaml
# Every field in one file. You would never use all of these at once.
name: kitchen-sink
version: "1"

# Pipeline-level settings
vars:
  repo: "8op-org/gl1tch"
  max_lines: "500"
max_parallel: 4

steps:
  # Step 1: Fetch data with a sidecar executor
  - id: fetch-issues
    executor: gh
    vars:
      args: "issue list --repo {{vars.repo}} --json title,body --limit 10"

  # Step 2: AI analysis with dependency
  - id: analyze
    executor: claude
    model: claude-sonnet-4-6
    needs: [fetch-issues]
    prompt: |
      Analyze these GitHub issues and categorize by priority:
      {{steps.fetch-issues.output}}
    outputs:
      summary: ""

  # Step 3: Conditional branching
  - id: check-urgency
    executor: builtin.assert
    needs: [analyze]
    args:
      expected: "critical"
      actual: "{{steps.analyze.output}}"
    condition:
      if: "contains:critical"
      then: alert
      else: log-ok

  # Step 4a: Alert path
  - id: alert
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [analyze]
    prompt: "Draft an urgent alert for: {{steps.analyze.summary}}"

  # Step 4b: Quiet path
  - id: log-ok
    executor: builtin.log
    needs: [analyze]
    args:
      message: "No critical issues found."

  # Step 5: Write output to file
  - id: save-report
    executor: write
    needs: [analyze]
    vars:
      path: "./issue-report.md"
    input: "{{steps.analyze.output}}"

  # Step 6: Retry on failure
  - id: post-webhook
    executor: builtin.http_get
    needs: [analyze]
    args:
      url: "https://hooks.example.com/notify"
    retry:
      max_attempts: 3
      interval: "5s"
      on: on_failure
    on_failure: log-webhook-error

  # Fallback step
  - id: log-webhook-error
    executor: builtin.log
    args:
      message: "Webhook delivery failed after 3 attempts."
```
