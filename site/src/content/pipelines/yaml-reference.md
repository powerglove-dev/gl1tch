## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[polish] [type:finding title:yaml-reference.md was missing template functions tags:docs,template] The yaml-reference.md only documented {{steps.*}} and {{vars.*}} expressions. The full Go text/template function library (default, env, get, trim, upper, lower, replace, split, join, contains, toJson, fromJson, catLines, splitLines) defined in internal/pipeline/template.go was completely undocumented. Also missing: {{.param.input}} for --input flag values, and the hyphen-in-step-ID workaround using the get function. Fixed in site/src/content/pipelines/yaml-reference.md.
[scan_docs] ` blocks. If one is found and a store is available, it's persisted for the current run. Every subsequent step receives accumulated brain notes in its prompt preamble — automatically, before your prompt text.

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

The yaml-reference.md now documents the full template function library. Key additions:

- **Template functions table** — all 14 functions (`default`, `env`, `get`, `trim`, `upper`, `lower`, `trimPrefix`, `trimSuffix`, `replace`, `split`, `join`, `contains`, `hasPrefix`/`hasSuffix`, `catLines`, `splitLines`, `toJson`, `fromJson`) sourced directly from `template.go:19-62`
- **`{{.param.input}}`** — documents the `--input` flag value available at runtime
- **Hyphen workaround note** — explains why `{{get "steps.ask-llama.output" .}}` is needed for step IDs with hyphens
- **Pipe example** — shows `| trim | upper | default` chaining

<brain type="finding" tags="docs,template" title="yaml-reference.md was missing template functions">
The yaml-reference.md only documented {{steps.*}} and {{vars.*}} expressions. The full Go text/template function library (default, env, get, trim, upper, lower, replace, split, join, contains, toJson, fromJson, catLines, splitLines) defined in internal/pipeline/template.go was completely undocumented. Also missing: {{.param.input}} for --input flag values, and the hyphen-in-step-ID workaround using the get function. Fixed in site/src/content/pipelines/yaml-reference.md.
</brain>

