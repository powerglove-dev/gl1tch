---
title: "The Brain System"
description: "Give your assistant memory — it records what it learns in one step and carries that knowledge into every step that follows."
order: 4
---

Your assistant reads, analyzes, and moves on — but without memory, every step starts from scratch. The brain system fixes that. When a step records a `<brain>` block, gl1tch stores it and injects it into every step that runs after. Your assistant remembers what it found so it can act on it later, automatically.


## Quick Start

Add one instruction to your prompt. Your assistant does the rest.

```yaml
name: security-audit
version: "1"

steps:
  - id: audit
    executor: ollama
    model: qwen2.5-coder:latest
    prompt: |
      Audit this codebase for security issues.
      Record your top findings in a <brain> block at the end.

  - id: report
    executor: claude
    model: claude-sonnet-4-6
    needs: [audit]
    prompt: |
      Using the brain context from the audit above,
      write a prioritized remediation plan.
```

Run it:

```bash
glitch pipeline run security-audit.pipeline.yaml
```

The `report` step receives the audit findings automatically — no template expressions, no copy-paste between steps.


## How It Works

Tell your assistant to emit a `<brain>` block. gl1tch finds it, stores it, and injects it into every later step's prompt before your text runs.

Your assistant outputs its analysis, then appends:

```text
<brain tags="security,sql-injection">
SQL injection in user_search (line 42), admin_filter (line 89),
report_query (line 156). All use string concatenation. No parameterized queries.
</brain>
```

The next step sees this before your prompt text:

```text
## Brain Notes (this run)

[audit] [tags: security, sql-injection]
SQL injection in user_search (line 42), admin_filter (line 89),
report_query (line 156). All use string concatenation. No parameterized queries.

---

[your prompt follows here]
```

No configuration. Brain scanning is always on.


## Writing to the Brain

Your prompt is the only control you have — and it's all you need. Tell your assistant what to record, and it will emit a `<brain>` block.

```yaml
prompt: |
  Find patterns in these GitHub issues. What areas keep breaking?
  Record your top 3 findings in a <brain> block.

  {{steps.fetch.output}}
```

The full output — including the brain block — still goes to stdout. gl1tch extracts and stores the block separately.


## Brain Block Format

```xml
<brain tags="tag1,tag2" type="finding" title="Short Title">
  The insight. Specific. One or two sentences.
</brain>
```

All attributes are optional:

| Attribute | Purpose |
|-----------|---------|
| `tags` | Comma-separated labels for grouping related notes |
| `type` | `research`, `finding`, `data`, or `code` |
| `title` | Short label shown in the injected preamble header |

Keep brain notes short. A focused two-sentence finding is useful. Four paragraphs is noise.


## Telling Your Assistant the Context Is There

Brain notes arrive before your prompt text automatically. Your assistant responds better when you mention them explicitly:

```text
Using the brain context from earlier steps in this run,
summarize the findings and recommend next actions.
```

Not required. Noticeably better when included.


## Brain vs. `{{steps.<id>.output}}`

These two tools do different things. Use the right one for the job.

| Use | When |
|-----|------|
| `{{steps.fetch.output}}` | You need raw data — JSON, a diff, a file list |
| Brain | You want what your assistant *understood*, not what it *received* |

The brain carries interpreted signal. Template expressions carry raw data. A good pipeline uses both.


## How Long Brain Notes Live

Within a single run, notes written by earlier steps inject into later steps automatically. That's the primary use case.

Notes also persist to your assistant's long-term store. A brain note from last Tuesday is available for semantic retrieval in a pipeline you run today, when a RAG injector is configured.


## Examples


### Morning Issue Triage

A fast-and-smart handoff: your local model finds patterns, Claude acts on them.

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

The `recommend` step never references `{{steps.analyze.output}}` — the brain notes are already in its preamble.


### Multi-Layer Code Review

Each layer records what it found. The final step synthesizes everything.

```yaml
name: layered-review
version: "1"

steps:
  - id: security
    executor: ollama
    model: qwen2.5-coder:latest
    prompt: |
      Review this diff for security issues only.
      Record your findings in a <brain> block tagged "security".

      {{steps.diff.output}}

  - id: performance
    executor: ollama
    model: qwen2.5-coder:latest
    needs: [security]
    prompt: |
      Review this diff for performance issues only.
      Record your findings in a <brain> block tagged "performance".

      {{steps.diff.output}}

  - id: summary
    executor: claude
    model: claude-sonnet-4-6
    needs: [performance]
    prompt: |
      Using all brain context from this run, write a final
      code review that combines security and performance findings.
```


## Reference

| Concept | Detail |
|---------|--------|
| Brain scanning | Always on when running via `glitch pipeline run` |
| Storage | Persisted to your assistant's SQLite store |
| Injection timing | Before your prompt text in every subsequent step |
| Scope within a run | All steps after the writing step receive the note |
| Scope across runs | Available for semantic retrieval via RAG injector |
| Max useful length | Two to four sentences per brain block |


## See Also

- [Pipelines](/docs/pipelines/pipelines) — step execution, input/output, and variable interpolation
- [Prompts](/docs/pipelines/prompts) — save reusable instructions alongside brain-aware pipelines
- [Cron](/docs/pipelines/cron) — schedule brain-powered pipelines to run while you sleep
