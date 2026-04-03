---
title: "Pipeline YAML Reference"
description: "Look up every pipeline field, condition, retry, and template expression."
order: 2
---

Complete field reference for `.pipeline.yaml` files. Every field your pipeline can use is in the tables below.

## Pipeline Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | `string` | *required* | Unique pipeline name. Used in logs and as the lookup key for `glitch pipeline run`. |
| `version` | `string` | `""` | Schema version. Always `"1"`. |
| `description` | `string` | `""` | What the pipeline does. Used by `glitch ask` to route natural-language prompts to the right pipeline. |
| `steps` | `Step[]` | *required* | Ordered list of steps. Run order is determined by `needs` dependencies, not array position. |
| `vars` | `map[string]any` | `{}` | Pipeline-level variables. Available to all steps as `{{vars.<name>}}`. |
| `max_parallel` | `int` | `8` | Maximum steps running at the same time. Only matters when steps have parallel `needs` branches. |

## Step Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | `string` | *required* | Unique step identifier. Referenced by `needs`, `on_failure`, and template expressions. |
| `executor` | `string` | `""` | What runs this step. AI: `claude`, `ollama`. Builtins: `builtin.assert`, `builtin.log`, etc. Tools: `gh`, `jq`, `write`, or any name in `~/.config/glitch/wrappers/`. |
| `model` | `string` | `""` | Model passed to the executor. For `claude`: `claude-sonnet-4-6`. For `ollama`: `qwen2.5-coder:latest`. |
| `prompt` | `string` | `""` | Prompt sent to AI executors. Supports `{{template}}` expressions. |
| `input` | `string` | `""` | Raw input string for non-AI executors, or to override the default input. Supports template expressions. |
| `publish_to` | `string` | `""` | Topic name. When set, this step's output is published to the event bus under that topic. |
| `needs` | `string[]` | `[]` | Step IDs that must complete before this step starts. Defines the execution graph. |
| `vars` | `map[string]string` | `{}` | Key-value pairs passed to the executor. For `gh` and `jq`, use `vars.args` to pass CLI arguments. |
| `args` | `map[string]any` | `{}` | Structured data for builtins that need typed input. Supersedes `vars` when set. |
| `condition` | `Condition` | `{}` | Branch execution based on the previous step's output. See [Conditions](#conditions). |
| `for_each` | `string` | `""` | Template expression or newline-separated list. The step is cloned once per item; use `{{item}}` for the current value. |
| `retry` | `RetryPolicy` | `null` | Retry configuration for this step. See [Retry](#retry). |
| `on_failure` | `string` | `""` | Step ID to run if this step fails after all retries. |
| `prompt_id` | `string` | `""` | Title of a saved prompt in the store. That prompt body is prepended to the step's input before execution. |
| `outputs` | `map[string]string` | `{}` | Output keys produced by this step. After completion, accessible as `{{steps.<id>.<key>}}`. |
| `inputs` | `map[string]string` | `{}` | Maps input names to template expressions like `{{steps.<id>.<key>}}`. Resolved before execution. |
| `write_brain` | `bool` | `false` | When true, this step's output is written to the brain store. Future runs can pull it in with `--brain`. |

## Conditions

The `condition` field branches execution based on the previous step's output.

| Field | Type | Description |
|-------|------|-------------|
| `if` | `string` | Expression to evaluate against the previous step's output. |
| `then` | `string` | Step ID to jump to when the expression is true. |
| `else` | `string` | Step ID to jump to when the expression is false. |

### Condition Expressions

| Expression | Matches when |
|------------|-------------|
| `always` | Always true. |
| `not_empty` | Output is not empty after trimming whitespace. |
| `contains:<str>` | Output contains the literal string `<str>`. |
| `matches:<regex>` | Output matches the regex pattern `<regex>`. |
| `len > N` | Output length in bytes exceeds `N`. |

```yaml
- id: classify
  executor: claude
  model: claude-haiku-4-5-20251001
  prompt: "Is this Go or Python? Reply with one word."
  condition:
    if: "contains:Go"
    then: go-handler
    else: python-handler
```

## Retry

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | `int` | `0` | Number of retry attempts. `0` means no retry. |
| `interval` | `duration` | `"0s"` | Time between retries. Accepts `"2s"`, `"500ms"`, `"1m"`. |
| `on` | `string` | `"always"` | `"always"` retries on any completion. `"on_failure"` retries only on error. |

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

## Template Expressions

Templates use `{{double braces}}` and are resolved before each step runs.

| Expression | Resolves to |
|------------|-------------|
| `{{steps.<id>.output}}` | The string output of a completed step. |
| `{{steps.<id>.<key>}}` | A specific output key declared in that step's `outputs` map. |
| `{{vars.<name>}}` | A pipeline-level variable from the `vars` map. |
| `{{item}}` | The current item inside a `for_each` loop. |

## Complete Example

Every field in one file. You would never use all of these at once.

```yaml
name: kitchen-sink
version: "1"
vars:
  repo: "8op-org/gl1tch"
max_parallel: 4

steps:
  # Fetch data with a CLI tool
  - id: fetch-issues
    executor: gh
    vars:
      args: "issue list --repo {{vars.repo}} --json title,body --limit 10"

  # AI analysis that depends on fetch
  - id: analyze
    executor: claude
    model: claude-sonnet-4-6
    needs: [fetch-issues]
    prompt: |
      Categorize these GitHub issues by priority:
      {{steps.fetch-issues.output}}
    outputs:
      summary: ""

  # Branch based on the analysis output
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

  # Alert path
  - id: alert
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [analyze]
    prompt: "Draft an urgent alert for: {{steps.analyze.summary}}"

  # Quiet path
  - id: log-ok
    executor: builtin.log
    needs: [analyze]
    args:
      message: "No critical issues found."

  # Write output to disk
  - id: save-report
    executor: write
    needs: [analyze]
    vars:
      path: "./issue-report.md"
    input: "{{steps.analyze.output}}"

  # Retry on failure
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

  - id: log-webhook-error
    executor: builtin.log
    args:
      message: "Webhook delivery failed after 3 attempts."
```

## See Also

- [Executors](/docs/pipelines/executors) — what each executor does and how to configure it
- [CLI Reference](/docs/pipelines/cli-reference) — running pipelines from the command line
- [Workflows](/docs/pipelines/workflows) — chain multiple pipelines together
