---
title: "The Brain System"
description: "Steps write what they learn. Later steps read it. The model decides what's worth remembering."
order: 4
---

## How it works

Every step's output is scanned for `<brain>` blocks. If one is found and a store is available, it's persisted for the current run. Every subsequent step receives accumulated brain notes in its prompt preamble — automatically, before your prompt text.

There are no YAML flags that control this. Brain scanning and injection are always on when a store is configured (which is the default when running via `glitch pipeline run`). The model decides what's worth remembering by whether it emits a `<brain>` block.

## Writing to the brain

Your prompt instructs the model to emit a brain block. gl1tch finds it, extracts it, stores it.

```yaml
steps:
  - id: audit
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Audit this codebase for security issues. Be specific.
      Record your key findings in a <brain> block at the end.
```

The model outputs its analysis, then appends:

```
<brain tags="security,sql-injection">
SQL injection in user_search (line 42), admin_filter (line 89),
report_query (line 156). All use string concatenation. No parameterized queries.
</brain>
```

gl1tch extracts the block, stores it, moves on. The full output — including the brain block — still goes to stdout.

## Reading from the brain

If brain notes exist for the current run, they appear in the next step's prompt preamble automatically. No opt-in needed. The model sees something like this before your prompt text:

```
## Brain Notes (this run)

[audit] [tags: security, sql-injection]
SQL injection in user_search (line 42), admin_filter (line 89),
report_query (line 156). All use string concatenation. No parameterized queries.

---
```

Then your prompt follows. The model sees both.

## The brain block format

```xml
<brain tags="tag1,tag2" type="finding" title="Short Title">
  The insight. Specific. One or two sentences.
</brain>
```

All attributes are optional:

| Attribute | What it's for |
|-----------|--------------|
| `tags` | Comma-separated labels. Useful for filtering later. |
| `type` | `research`, `finding`, `data`, or `code`. |
| `title` | Short label shown in the preamble header. |

Keep brain notes short. A four-sentence insight is useful. Four paragraphs is noise.

## A real example

This pipeline fetches open issues, has a local model find patterns, then has Claude synthesize recommendations. The brain carries the local model's findings to Claude automatically.

```yaml
name: issue-triage
version: "1"

steps:
  - id: fetch
    executor: gh
    vars:
      args: "issue list --json title,body,labels --limit 20"

  - id: analyze
    executor: ollama
    model: qwen2.5-coder:latest
    needs: [fetch]
    prompt: |
      Find patterns in these issues. What areas of the codebase
      keep breaking? Record your top 3 findings in a <brain> block.

      {{steps.fetch.output}}

  - id: recommend
    executor: claude
    model: claude-sonnet-4-6
    needs: [analyze]
    prompt: |
      Using the brain context from the pattern analysis above,
      suggest which issues to prioritize this sprint and why.
```

Step 3 never references `{{steps.analyze.output}}`. It doesn't need to — the brain notes from step 2 are already in its preamble.

## Brain vs. `{{steps.<id>.output}}`

Use `{{steps.<id>.output}}` when you need the full, raw output of a step — JSON, a diff, a file list.

Use the brain when you want a step to record what it *understood*, not what it *received*. The brain carries interpreted signal. Template expressions carry raw data.

A good pipeline uses both.

## Tell the model the context is there

The preamble arrives before your prompt, but models respond better when you mention it explicitly:

```
Using the brain context from earlier steps in this run,
summarize the findings and recommend next actions.
```

Not required. Noticeably better when included.

## How long brain notes live

Brain notes are written to the gl1tch SQLite store and stay there. They don't get deleted between runs.

Within a single run, notes written by earlier steps are injected into later steps automatically. That's the primary use case.

Across runs, all accumulated brain notes are indexed into the RAG store — the same vector store used by `builtin.index_code`. So a brain note written by a pipeline run last Tuesday is available for semantic retrieval in a pipeline you run today, as long as the RAG injector is configured.
