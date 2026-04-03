---
title: "Executors"
description: "Configure the AI providers and tools that run your pipeline steps."
order: 3
---

Every pipeline step has an `executor` field. That field picks what does the work — an AI model, a CLI tool, or a lightweight builtin. Here's how to configure each one.

## Quick Start

Pick your provider and copy the YAML:

**Local AI (Ollama):**
```yaml
- id: analyze
  executor: ollama
  model: qwen2.5-coder:latest
  prompt: "Explain what this function does."
```

**Claude:**
```yaml
- id: review
  executor: claude
  model: claude-sonnet-4-6
  prompt: "Review this code for bugs."
```

**GitHub CLI:**
```yaml
- id: list-prs
  executor: gh
  vars:
    args: "pr list --json number,title,author"
```

## AI Providers

### ollama

Runs a model on your local [Ollama](https://ollama.ai) instance. Connects to `localhost:11434`. No API key. No data leaves your machine.

```yaml
- id: analyze
  executor: ollama
  model: qwen2.5-coder:latest
  prompt: "What does this function do?\n\n{{steps.fetch.output}}"
```

Pull the model first:

```bash
ollama pull qwen2.5-coder:latest
```

The `model` field is the exact Ollama model tag. Use any model you have pulled.

### claude

Runs Claude via the Claude CLI. Install and authenticate the CLI before using this executor — gl1tch delegates to it.

```yaml
- id: review
  executor: claude
  model: claude-sonnet-4-6
  prompt: |
    Review this pull request diff for correctness and security.
    {{steps.get-diff.output}}
```

Use the full model identifier in the `model` field.

| Model | Description |
|-------|-------------|
| `claude-sonnet-4-6` | Best quality. Use for complex analysis and writing. |
| `claude-haiku-4-5-20251001` | Faster. Use for classification, formatting, and simple transforms. |

## Provider Table

| Executor | Runs where | Needs |
|----------|-----------|-------|
| `ollama` | Your machine | Ollama running locally, model pulled |
| `claude` | Anthropic API | Claude CLI installed and authenticated |

> **TIP:** Use `ollama` for routing decisions and quick transforms. Use `claude` for tasks that benefit from a larger model — code review, complex reasoning, writing.

## CLI Tool Executors

These wrap command-line tools. They pass `vars.args` to the tool and capture stdout.

### gh

GitHub CLI wrapper. Requires the [GitHub CLI](https://cli.github.com/) installed and authenticated.

```yaml
- id: list-prs
  executor: gh
  vars:
    args: "pr list --json number,title,author"
```

Pass any `gh` subcommand and flags as `vars.args`. Output is captured and available to downstream steps.

### jq

Runs [jq](https://jqlang.github.io/jq/) on the step input. Use it to transform JSON between steps.

```yaml
- id: extract-titles
  executor: jq
  needs: [list-prs]
  input: "{{steps.list-prs.output}}"
  vars:
    args: ".[].title"
```

### write

Writes the step input to a file on disk.

```yaml
- id: save-report
  executor: write
  needs: [review]
  vars:
    path: "./review.md"
  input: "{{steps.review.output}}"
```

### Custom tool wrappers

Wrap any CLI tool by creating a YAML file in `~/.config/glitch/wrappers/`:

```yaml
# ~/.config/glitch/wrappers/my-tool.yaml
name: my-tool
description: "Custom CLI integration"
command: /usr/local/bin/my-tool
args: ["--format", "json"]
kind: tool
```

Now use `executor: my-tool` in any pipeline. Input goes to stdin, output comes from stdout. Each key in `vars` is passed to the process as `GLITCH_<KEY>=<value>`.

The `kind` field controls what context the executor receives:
- `tool` — receives only the step input
- `agent` — receives the full execution context (default)

## Builtins

Builtins are lightweight functions built into gl1tch. They don't spawn processes or call APIs. Use them for control flow, validation, and glue.

### builtin.assert

Validates a value. Fails the step if the assertion is false. Use it to gate downstream steps on expected output.

```yaml
- id: check
  executor: builtin.assert
  needs: [analyze]
  args:
    expected: "pass"
    actual: "{{steps.analyze.output}}"
```

### builtin.set_data

Sets static values in the step output. Useful for injecting constants or seed data into the pipeline.

```yaml
- id: config
  executor: builtin.set_data
  args:
    data:
      repo: "8op-org/gl1tch"
      branch: "main"
```

### builtin.log

Logs a message during pipeline execution. Does not produce output for downstream steps.

```yaml
- id: checkpoint
  executor: builtin.log
  needs: [fetch]
  args:
    message: "Fetch complete. Starting analysis."
```

### builtin.sleep

Pauses execution for a duration. Useful for rate limiting.

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

Indexes a codebase directory for semantic search using a local model. Run this as a first step before asking questions about a codebase.

```yaml
- id: index
  executor: builtin.index_code
  args:
    root: "."
    model: "qwen2.5-coder:latest"
```

## Full Example: Code Review Pipeline

Four steps, three executor types:

```yaml
name: code-review
version: "1"
steps:
  # Fetch the PR diff with the GitHub CLI
  - id: get-diff
    executor: gh
    vars:
      args: "pr diff 42"

  # AI review
  - id: review
    executor: claude
    model: claude-sonnet-4-6
    needs: [get-diff]
    prompt: |
      Review this pull request diff. Focus on correctness, edge cases,
      and security. Be specific.

      {{steps.get-diff.output}}

  # Log a checkpoint
  - id: log-done
    executor: builtin.log
    needs: [review]
    args:
      message: "Review complete."

  # Save the review to disk
  - id: save
    executor: write
    needs: [review]
    vars:
      path: ./review-pr42.md
    input: "{{steps.review.output}}"
```

## See Also

- [Pipeline YAML Reference](/docs/pipelines/yaml-reference) — every field and what it does
- [CLI Reference](/docs/pipelines/cli-reference) — running pipelines from the command line
- [Workflows](/docs/pipelines/workflows) — chain multiple pipelines together
