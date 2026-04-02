---
title: "Prompts"
description: "Save and test reusable prompts, inject them into pipelines, and iterate interactively."
order: 99
---

Prompts are persistent, searchable templates for AI interactions. gl1tch lets you author prompts once, test them interactively against live models, and inject them into pipeline steps—eliminating copy-paste, enabling fast iteration, and building a library of proven instructions.

## Architecture

The prompt system has three layers: **storage** (SQLite table), **authoring** (interactive TUI modal), and **injection** (pipeline step field).

Prompts live in the `prompts` table in gl1tch's SQLite database. When you create or edit a prompt via the prompt manager TUI (`internal/promptmgr/`), changes persist immediately. Pipeline steps reference saved prompts by `prompt_id`; during execution, the `internal/pipeline/runner.go` resolves the prompt body from the store and prepends it to the step's input before execution.

The prompt manager is a full-screen modal accessible from the jump window (press `j` then type "prompts") or via a configurable keybinding. It mirrors the architecture of other gl1tch modals: a BubbleTea model with independent panels for list navigation, editing, and live test output.


## Technologies

- **SQLite** — Persistent storage with additive schema migrations; prompts are queried by ID or title.
- **BubbleTea** — TUI framework for the three-panel prompt manager modal.
- **Streaming executor** — The test runner panel streams model responses into a scrollable viewport without blocking the TUI.


## Concepts

**Prompt ID** — The unique identifier for a saved prompt. Used in pipeline `prompt_id` fields to reference the prompt.

**Prompt Title** — Human-readable name (e.g., "Summarize code review"). Titles are displayed in the prompt manager list and fuzzy-searchable.

**Prompt Body** — The instruction text sent to the model. When a prompt is injected via `prompt_id`, the full body is prepended to the step's input.

**Model Slug** — The executor/model name (e.g., "openai", "ollama:mistral") associated with the prompt. The prompt manager shows this in the list so you can filter or browse prompts by their intended model.

**Prompt Test Runner** — The inline panel in the prompt manager where you run a prompt against a selected model in real-time, view streamed output, and refine the prompt without leaving the TUI.

**Fuzzy Search** — In the prompt manager list, press `/` to filter prompts by title or model slug. Search is incremental and case-insensitive.


## Configuration / YAML Reference

### Pipeline Step `prompt_id` Field

Add an optional `prompt_id` field to any pipeline step that executes an agent or model call:

```yaml
steps:
  - id: generate_code
    executor: openai
    model: gpt-4
    prompt_id: "Code generation best practices"
    input: "Generate a function that..."
```

When the step runs, gl1tch resolves the prompt body from the store and prepends it to `input`. The executor receives:

```
[prompt body from store]

[step input]
```

**Fields:**
- `prompt_id` (string, optional) — Title of the saved prompt to inject. If the prompt does not exist in the store, the step fails with a descriptive error. If omitted, step execution is identical to current behavior.

**Behavior:**
- Prompt body is prepended *before* any other input manipulation.
- Brain context injection and RAG happen *after* prompt injection.
- If `prompt_id` is set but empty, it is treated as omitted (no error).


## Examples

### Create a Prompt in the TUI

1. Press `j` to open the jump window.
2. Type "prompts" and press Enter.
3. Press `n` (or the bound key) to create a new prompt.
4. Enter title: "Code review synthesis"
5. Enter body:
   ```
   You are an expert code reviewer. Synthesize the key findings from code review comments.
   Focus on:
   - Security issues
   - Performance bottlenecks
   - Maintainability concerns
   ```
6. Select model: `ollama:mistral` (or your configured executor).
7. Press `t` to test the prompt against a sample input.
8. Press `s` to save.

### Inject a Prompt into a Pipeline

```yaml
name: "weekly_codebase_review"
steps:
  - id: fetch_diffs
    executor: shell
    command: "git diff main --stat | head -20"

  - id: analyze_changes
    executor: ollama
    model: "mistral"
    prompt_id: "Code review synthesis"
    input: |
      Analyze these recent code changes:
      {{get "fetch_diffs.data.output" .}}
    
  - id: write_summary
    executor: shell
    command: |
      cat > weekly_review.md << 'EOF'
      {{get "analyze_changes.data.output" .}}
      EOF
```

When `analyze_changes` runs, the prompt body ("You are an expert code reviewer...") is prepended to the `input` before sending to ollama.

### Use the Same Prompt Across Multiple Steps

```yaml
name: "multi_stage_review"
steps:
  - id: review_api
    executor: openai
    model: gpt-4
    prompt_id: "Code review synthesis"
    input: "Review the API layer"

  - id: review_database
    executor: openai
    model: gpt-4
    prompt_id: "Code review synthesis"
    input: "Review the database layer"
```

Both steps share the same prompt; edits to "Code review synthesis" in the prompt manager apply to both instantly.


## See Also

- [Pipelines](/docs/pipelines.md) — Step execution, input/output, and variable interpolation
- [Executors](/docs/executors.md) — Available models and how to configure them
- [Brain](/docs/brain.md) — How brain context and RAG injection work alongside prompts

