---
title: "Executors and Plugins"
description: "The workers that run your steps. AI models, tools, builtins."
order: 3
---

Every step in a pipeline has an `executor` field. That field tells the runner what does the work. There are three kinds of executors, and they all use the same field.

## Native executors

These are compiled into the gl1tch binary. No installation needed.

### claude

Runs Claude via the Claude CLI. Install and authenticate the CLI first — gl1tch delegates to it.

```yaml
- id: review
  executor: claude
  model: claude-sonnet-4-6
  prompt: "Review this code for bugs."
```

Supports all Claude models. Use the full model identifier in the `model` field: `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`, and so on.

### ollama

Uses a locally running [Ollama](https://ollama.ai) instance. The runner connects to `localhost:11434` by default.

```yaml
- id: analyze
  executor: ollama
  model: qwen2.5-coder:latest
  prompt: "Explain what this function does."
```

You need to have the model pulled first: `ollama pull qwen2.5-coder:latest`. The `model` field is the exact Ollama model tag.

## Sidecar executors (CLI wrappers)

Sidecar executors wrap any command-line tool. They're defined in YAML files in `~/.config/glitch/wrappers/` and loaded at startup. The gl1tch binary discovers them automatically.

A sidecar YAML file looks like this:

```yaml
name: gh
description: "GitHub CLI wrapper"
command: gh
args: []
kind: tool
```

When you use `executor: gh` in a pipeline, the runner finds this sidecar, spawns the `gh` process, passes input via stdin, and captures stdout.

### gh

GitHub CLI wrapper. Pass the gh command as `vars.args`:

```yaml
- id: list-prs
  executor: gh
  vars:
    args: "pr list --json number,title,author"
```

Requires the [GitHub CLI](https://cli.github.com/) installed and authenticated via `gh auth login`.

### jq

Runs [jq](https://jqlang.github.io/jq/) on the input. Useful for transforming JSON between steps:

```yaml
- id: extract-names
  executor: jq
  needs: [list-prs]
  input: "{{steps.list-prs.output}}"
  vars:
    args: ".[].title"
```

### write

Writes the step input to a file:

```yaml
- id: save
  executor: write
  needs: [review]
  vars:
    path: "./output.md"
  input: "{{steps.review.output}}"
```

### Custom sidecars

You can wrap any CLI tool. Create a YAML file in `~/.config/glitch/wrappers/`:

```yaml
# ~/.config/glitch/wrappers/my-tool.yaml
name: my-tool
description: "Custom CLI integration"
command: /usr/local/bin/my-tool
args: ["--format", "json"]
kind: tool
```

Now use `executor: my-tool` in any pipeline. Input goes to stdin, output comes from stdout. Environment variables are set as `GLITCH_<KEY>=<value>` from the step's `vars` map.

The `kind` field matters: `tool` executors receive only the step input. `agent` executors (the default) receive the full execution context.

## Builtins

Builtins are lightweight functions compiled into the runner. They don't spawn processes or call APIs. Use them for control flow, validation, and glue logic.

### builtin.assert

Validates output. Fails the step if the assertion is false.

```yaml
- id: check
  executor: builtin.assert
  needs: [analyze]
  args:
    expected: "pass"
    actual: "{{steps.analyze.output}}"
```

### builtin.set_data

Sets static data in the step output. Useful for injecting constants or seed values into the DAG.

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

Pauses execution for a duration. Useful for rate limiting or waiting on external processes.

```yaml
- id: wait
  executor: builtin.sleep
  args:
    duration: "5s"
```

### builtin.http_get

Makes a GET request to a URL and returns the response body.

```yaml
- id: fetch-data
  executor: builtin.http_get
  args:
    url: "https://api.example.com/status"
    timeout: "10s"
```

### builtin.index_code

Indexes a codebase directory for semantic search. Uses an Ollama model for embeddings. Useful as a first step before asking questions about code.

```yaml
- id: index
  executor: builtin.index_code
  args:
    root: "."
    model: "qwen2.5-coder:latest"
```

## Putting it together

Here's a real pipeline that uses multiple executor types in one flow -- a code review pipeline:

```yaml
name: code-review
version: "1"
steps:
  # Sidecar executor: fetch the PR diff
  - id: get-diff
    executor: gh
    vars:
      args: "pr diff 42"

  # Native executor: AI reviews the diff
  - id: review
    executor: claude
    model: claude-sonnet-4-6
    needs: [get-diff]
    prompt: |
      Review this pull request diff. Focus on correctness, edge cases,
      and security. Be specific.

      {{steps.get-diff.output}}

  # Builtin: log a checkpoint
  - id: log-done
    executor: builtin.log
    needs: [review]
    args:
      message: "Review complete."

  # Sidecar executor: write the review to disk
  - id: save
    executor: write
    needs: [review]
    vars:
      path: ./review-pr42.md
    input: "{{steps.review.output}}"
```

Four steps, three different executor types, one pipeline. The runner handles the dependency graph, streams output, and stores everything in the local database.
