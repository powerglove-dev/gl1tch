---
title: "Executors"
description: "The AI providers and tools that run your pipeline steps."
order: 3
---

Every pipeline step has an `executor` field that picks what does the work — a local AI model, a cloud provider, a CLI tool, or a builtin. You can ask gl1tch to build pipeline steps for you, or write them directly in YAML.

## Quick reference

| Executor | What it does | Needs |
|----------|-------------|-------|
| `ollama` | Runs a local model via Ollama | Ollama running, model pulled |
| `claude` | Runs Claude via the Claude CLI | Claude CLI authenticated |
| `shell` | Runs any shell command | Nothing extra |
| `gh` | Runs a `gh` CLI subcommand | GitHub CLI authenticated |
| `jq` | Applies a jq filter to JSON input | `jq` on PATH |
| `builtin.*` | Lightweight built-in functions | Nothing extra |

## AI executors

### ollama

Runs a model on your local [Ollama](https://ollama.ai) instance at `localhost:11434`. No API key. No data leaves your machine.

```yaml
- id: summarize
  executor: ollama
  model: qwen2.5:latest
  prompt: |
    Summarize these commits in 2–3 sentences:
    {{.step.fetch.data.value}}
```

Pull the model first:

```bash
ollama pull qwen2.5:latest
```

Use `model` to set the Ollama model tag. You can use any model you have pulled — run `ollama list` to see what's available.

> **Note:** Smaller models (qwen2.5, llama3.2) handle summarization, classification, and generation well. Pipelines that use tools, run shell commands autonomously, or coordinate multi-step reasoning require a larger local model with tool/function-calling support — `ollama pull qwen3:8b` is a good starting point for agentic steps.

### claude

Runs Claude via the [Claude CLI](https://claude.ai/download). Install and authenticate the CLI before using this executor.

```yaml
- id: review
  executor: claude
  model: claude-haiku-4-5-20251001
  prompt: |
    Review this diff for correctness and security:
    {{.step.fetch.data.value}}
```

| Model | Use for |
|-------|---------|
| `claude-haiku-4-5-20251001` | Summarization, formatting, simple transforms (default) |
| `claude-sonnet-4-6` | Complex reasoning, code review, writing |
| `claude-opus-4-6` | Hardest tasks |

When no `model` is specified, gl1tch uses Haiku.

## Shell and CLI executors

### shell

Runs any shell command and captures stdout. Use this to fetch data for downstream AI steps.

```yaml
- id: fetch
  executor: shell
  vars:
    cmd: "git log --oneline -10"
```

Pass the command as `vars.cmd`. It runs inside `sh -c`.

### gh

Wraps the [GitHub CLI](https://cli.github.com/). Requires `gh` installed and authenticated.

```yaml
- id: fetch-diff
  executor: gh
  vars:
    args: "pr diff 42"
```

Pass any `gh` subcommand and flags as `vars.args`. Output is captured and available to downstream steps.

### jq

Applies a [jq](https://jqlang.github.io/jq/) filter to JSON input.

```yaml
- id: extract-titles
  executor: jq
  needs: [fetch-prs]
  input: "{{.step.fetch-prs.data.value}}"
  vars:
    filter: ".[].title"
```

Pass the filter expression as `vars.filter`. Input defaults to the previous step's output.

## Builtin executors

Builtins are lightweight functions built into gl1tch. They don't spawn processes or call external APIs — use them for control flow and glue between steps.

### builtin.assert

Fails the step — and halts the pipeline — if a value doesn't match what's expected.

```yaml
- id: check
  executor: builtin.assert
  needs: [analyze]
  args:
    expected: "pass"
    actual: "{{.step.analyze.data.value}}"
```

### builtin.set_data

Injects static values into the pipeline context. Useful for constants and seed data.

```yaml
- id: config
  executor: builtin.set_data
  args:
    data:
      repo: "8op-org/gl1tch"
      branch: "main"
```

### builtin.log

Logs a message during execution. Produces no output for downstream steps.

```yaml
- id: checkpoint
  executor: builtin.log
  needs: [fetch]
  args:
    message: "Fetch complete. Starting analysis."
```

### builtin.sleep

Pauses execution. Useful for rate limiting between steps.

```yaml
- id: wait
  executor: builtin.sleep
  args:
    duration: "5s"
```

### builtin.http_get

Makes a GET request and returns the response body.

```yaml
- id: fetch-status
  executor: builtin.http_get
  args:
    url: "https://api.example.com/status"
    timeout: "10s"
```

### builtin.index_code

Indexes a codebase for semantic search using a local model. Run this before `builtin.search_code` steps.

```yaml
- id: index
  executor: builtin.index_code
  args:
    root: "."
    model: "qwen2.5-coder:latest"
```

### builtin.search_code

Searches an indexed codebase semantically.

```yaml
- id: find
  executor: builtin.search_code
  needs: [index]
  args:
    query: "error handling in pipeline runner"
    limit: 5
```

## Passing output between steps

Use `{{.step.<id>.data.value}}` to inject a previous step's output into a prompt or input field:

```yaml
steps:
  - id: fetch
    executor: shell
    vars:
      cmd: "git log --oneline -5"

  - id: summarize
    executor: ollama
    model: qwen2.5:latest
    needs: [fetch]
    prompt: |
      Summarize these commits:
      {{.step.fetch.data.value}}
```

The `needs` field ensures `fetch` finishes before `summarize` starts.

## Custom wrappers

Wrap any CLI tool by dropping a YAML file in `~/.config/glitch/wrappers/`:

```yaml
# ~/.config/glitch/wrappers/my-tool.yaml
name: my-tool
description: "My custom CLI integration"
command: /usr/local/bin/my-tool
args: ["--format", "json"]
kind: tool
```

Use `executor: my-tool` in any pipeline step. Input goes to stdin. Each key in `vars` is passed as `GLITCH_<KEY>=<value>`. The `kind` field controls context:
- `tool` — receives only the step input
- `agent` — receives the full execution context (default)

## Full example: PR review pipeline

Three executor types working together — shell fetches the diff, Claude reviews it, shell saves the result:

```yaml
name: pr-review-local
version: "1"
steps:
  - id: fetch-diff
    executor: gh
    vars:
      args: "pr diff 42"

  - id: review
    executor: ollama
    model: qwen3:8b
    needs: [fetch-diff]
    prompt: |
      Review this pull request diff. Focus on correctness, edge cases,
      and security. Be specific.

      {{.step.fetch-diff.data.value}}

  - id: save
    executor: shell
    needs: [review]
    vars:
      cmd: "cat > review-pr42.md"
    input: "{{.step.review.data.value}}"
```

## See also

- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — every field and what it does
- [Examples](/docs/pipelines/examples) — ready-to-run pipelines for real workflows
- [Plugins](/docs/pipelines/plugins) — install community executors
