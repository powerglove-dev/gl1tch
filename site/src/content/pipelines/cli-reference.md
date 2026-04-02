## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[read_target] ` block.

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
[deep_search] `, supporting structured attributes (`type`, `tags`, `title`) so stored notes are semantically rich and RAG-queryable; keep `<brain_notes>` as a backward-compatible alias
- Make brain-writing **block-level opt-in**: any step output containing a `<brain>` block is parsed and stored regardless of `write_brain` flag — the agent decides what's worth remembering, not just the pipeline author
- Update the brain write instruction (injected into prompts) to document the richer `<brain>` format with examples
- Externalize four hardcoded system prompt constants to `~/.config/orcai/prompts/` as Markdown files installed on first run, loaded at runtime with embedded fallback

## Capabilities

### New Capabilities

- `brain-block-semantics`: Rich `<brain type="..." tags="..." title="...">` block format, updated parser, block-level opt-in storage, updated write instruction
- `user-configurable-prompts`: Install-on-first-run system prompts at `~/.config/orcai/prompts/`, runtime loader with embedded fallback, covering b

=== file:/Users/stokes/Projects/gl1tch/openspec/changes/pipeline-brain-context/design.md:L61-L73 ===
or emits malformed XML] → Mitigation: The runner treats `<brain>` parsing as best-effort. Missing or malformed blocks are silently skipped with a debug log line — the step does not fail. Future: add a `require_brain_write: true` flag that fails the step if no `<brain>` block is found.
- [Risk: Cross-run brain note contamination] → Mitigation: Phase 1 only injects notes written within the current pipeline run. A `run_id` column on `brain_notes` allows future scoping; notes from other runs are excluded from the preamble unless the pipeline sets `brain_scope: all`.
- [Risk: Removing `db` step breaks existing pipelines] → Mitigation: Grep confirms no current pipeline YAML uses `type: db`. The type is removed with no migration needed.
- [Risk: LLM generates SQL that reads sensitive data even with read-only intent] → Mitigation: Phase 1 brain preamble does not grant free SQL access. It only surfaces pre-fetched summaries assembled by `BrainInjector`. Raw query capability is a future opt-in.

## Migration Plan

1. Add `brain_notes` table via schema migration in `store/schema.go` — backward compatible, additive.
2. Add `BrainInjector` interface and `StoreBrainInjector` in `pipeline/brain.go`.
3. Extend `Pipeline` and `Step` structs with `UseBrain`/`WriteBrain` fields.
4. Extend `runner.go` prompt-building path to call the injector before dispatching.
5. Add `<brain>` block parser and `store.InsertBrainNote` call after step completion.
6. Delete `step_db.go`.
7. Update `pipeline.go`'s

=== file:/Users/stokes/Projects/gl1tch/openspec/changes/pipeline-brain-context/specs/pipeline-brain-read/spec.md:L1-L18 ===
## ADDED Requirements

### Requirement: Pipeline and Step structs support use_brain flag
The `Pipeline` struct SHALL include a `UseBrain bool` field (`yaml:"use_brain"`). The `Step` struct SHALL include a `UseBrain *bool` field (`yaml:"use_brain"`) using a pointer to allow tri-state (unset/true/false). A step with `use_brain: true` activates brain read injection for that step. A step with `use_brain: false` suppresses it even when the pipeline-level flag is `true`. A step with no `use_brain` field inherits the pipeline-level value.

#### Scenario: Pipeline-level use_brain activates for all agent steps
- **WHEN** a pipeline YAML sets `use_brain: true` at the top level and contains two agent steps with no step-level `use_brain` field
- **THEN** both steps receive the brain read pre-context preamble prepended to their prompt

#### Scenario: Step-level use_brain overrides pipeline-level
- **WHEN** a pipeline sets `use_brain: true` at the top level and one step sets `use_brain: false`
- **THEN** that step does NOT receive the brain preamble; other steps do

#### Scenario: use_brain false at pipeline level suppresses injection by default
- **WHEN** a pipeline has `use_brain: false` (or no `use_brain` field) and a step has no `use_brain` fi...[truncated]
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

GLITCH_CLARIFY: Which file should I write to or update — is this for `docs/brain.md`, `cli-reference.md`, or a new file in the docs directory? And do you have the current content of that file, or should I create it from scratch based on the specs and code shown?

