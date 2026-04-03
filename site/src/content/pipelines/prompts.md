---
title: "Prompts"
description: "Save the instructions you give your assistant most often — recall them instantly in any pipeline or live session."
order: 99
---

Some instructions you write once and never want to type again. Your code review persona. Your commit message style. Your preferred debugging approach. The prompts system is your personal library — save a prompt once, drop it into any pipeline with a single field, and update it in one place when you want to change it everywhere.


## Quick Start

Open the prompt manager:

```bash
glitch prompts
```

Press `n` to create a new prompt, give it a title and body, press `s` to save.

Use it in a pipeline:

```yaml
steps:
  - id: review
    executor: ollama
    model: qwen2.5-coder:latest
    prompt_id: "Code review persona"
    input: "{{steps.diff.output}}"
```

That's it. When the step runs, your saved prompt body prepends to the input automatically.


## Managing Your Prompt Library

### Create a prompt

1. Run `glitch prompts`
2. Press `n` — new prompt form opens
3. Enter a title: `"Code review persona"`
4. Enter the body:

```text
You are a senior engineer doing code review. Be direct and specific.
Focus on correctness first, then performance, then style.
Always suggest a fix, not just an observation.
```

5. Press `s` to save

### Find a prompt

Press `/` in the prompt manager to filter by title. Search is incremental and case-insensitive.

### Edit a prompt

Navigate to it, press `e`. Changes apply to every pipeline that references it — immediately, on next run.

### Test a prompt

Press `t` from the prompt manager. A test panel opens and streams a live response from your configured model. Iterate before you commit the prompt to a pipeline.


## Using Prompts in Pipelines

Add `prompt_id` to any step that calls a model. The value is the exact title of your saved prompt.

```yaml
steps:
  - id: review
    executor: ollama
    model: qwen2.5-coder:latest
    prompt_id: "Code review persona"
    input: |
      Review these changes:
      {{steps.diff.output}}
```

When this step runs, the executor receives:

```text
[your saved prompt body]

[your step input]
```

The saved prompt always comes first. Brain context and any other injections happen after.

> **NOTE:** If `prompt_id` references a title that doesn't exist in your library, the step fails with a clear error message. Use `glitch prompts` to verify the title before running.


## Examples


### Code Review Pipeline

Save a prompt titled `"Code review persona"` with your preferred reviewer voice, then reuse it across multiple pipelines.

```yaml
name: weekly-review
version: "1"

steps:
  - id: diff
    executor: shell
    command: "git diff main --stat | head -40"

  - id: review
    executor: ollama
    model: qwen2.5-coder:latest
    needs: [diff]
    prompt_id: "Code review persona"
    input: |
      Review these recent changes:
      {{steps.diff.output}}

  - id: save
    executor: shell
    needs: [review]
    command: |
      echo "{{steps.review.output}}" > review-$(date +%Y%m%d).md
```


### Same Prompt Across Multiple Steps

Both steps share the same persona. Edit the prompt once to update both.

```yaml
name: layered-review
version: "1"

steps:
  - id: review-api
    executor: claude
    model: claude-sonnet-4-6
    prompt_id: "Code review persona"
    input: "Review the API layer for correctness"

  - id: review-db
    executor: claude
    model: claude-sonnet-4-6
    prompt_id: "Code review persona"
    input: "Review the database layer for correctness"
```


### Commit Message Generator

Save a prompt titled `"Commit message style"` that describes your project's conventions. Use it any time you want consistent commit messages.

```yaml
name: commit-helper
version: "1"

steps:
  - id: diff
    executor: shell
    command: "git diff --cached"

  - id: message
    executor: ollama
    model: qwen2.5-coder:latest
    needs: [diff]
    prompt_id: "Commit message style"
    input: |
      Generate a commit message for this diff:
      {{steps.diff.output}}
```


## Reference

### Pipeline step fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt_id` | string | no | Title of the saved prompt to inject. Must match exactly. |

### Prompt manager keyboard shortcuts

| Key | Action |
|-----|--------|
| `n` | New prompt |
| `e` | Edit selected prompt |
| `t` | Test selected prompt against a model |
| `s` | Save |
| `/` | Filter/search by title |
| `q` | Close the manager |

### Injection order

When a step has `prompt_id` set, the executor receives content in this order:

1. Your saved prompt body
2. Your step's `input` field
3. Brain context (if any brain notes exist for this run)


## See Also

- [Brain](/docs/pipelines/brain) — combine saved prompts with memory for smarter pipelines
- [Pipelines](/docs/pipelines/pipelines) — full pipeline step reference
- [Cron](/docs/pipelines/cron) — schedule pipelines that use your prompt library
