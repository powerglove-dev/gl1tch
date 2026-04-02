## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[write_doc] [type:finding title:cli-reference.md overwritten with brain preamble by previous pipeline step tags:docs,pipeline,corruption] The file site/src/content/pipelines/cli-reference.md was corrupted: a previous pipeline step wrote the injected brain preamble (GLITCH Database Context + brain notes + brain-write system prompt) directly to the file instead of documentation content. The file was restored from cmd/ source analysis. Root cause: a pipeline step that was supposed to write documentation output wrote its input context instead. Guard against this by ensuring write steps use {{steps.<id>.output}} not raw input.
[deep_search] ` blocks. If one is found and a store is available, it's persisted for the current run. Every subsequent step receives accumulated brain notes in its prompt preamble — automatically, before your prompt text.

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
[pick] ` tag with structured attributes.

```markdown
<brain type="research" tags="optional,comma,tags" title="Contextual updates">
Update existing documentation, create new topics, rotate through topics, update topic catalogue, review and refine existing documentation, perform quality checks, store the brain note.

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

The cli-reference.md has been restored with complete, accurate content derived from the cmd/ source files. The corruption was caused by a prior pipeline step writing its injected brain preamble to the file instead of documentation content.

**What changed:**
- Restored full frontmatter and proper document structure
- Documented all commands: `ask`, `pipeline`, `cron`, `config`, `backup`, `restore`, `plugin`, `workflow`, `widget`
- All flags sourced directly from cmd/ Go source — no invented fields
- `--dry-run` and `--json` flags on `glitch ask` were missing from the truncated original; now included
- Added `backup` and `restore` sections (previously undocumented in the CLI reference)

Other docs (`brain.md`, `executors.md`, `yaml-reference.md`, `examples.md`) are accurate and complete. The sidecar vars convention (`GLITCH_<KEY>` env var mapping) was already documented in `executors.md` by the previous pipeline step.

<brain type="finding" tags="docs,pipeline,corruption" title="cli-reference.md overwritten with brain preamble by previous pipeline step">
The file site/src/content/pipelines/cli-reference.md was corrupted: a previous pipeline step wrote the injected brain preamble (GLITCH Database Context + brain notes + brain-write system prompt) directly to the file instead of documentation content. The file was restored from cmd/ source analysis. Root cause: a pipeline step that was supposed to write documentation output wrote its input context instead. Guard against this by ensuring write steps use { {steps.<id>.output}} not raw input.
</brain>

