## GLITCH Database Context

### Schema: runs table (read-only)
Columns: id (INTEGER PK), kind (TEXT), name (TEXT), started_at (INTEGER unix-ms),
finished_at (INTEGER unix-ms, nullable), exit_status (INTEGER, nullable),
stdout (TEXT), stderr (TEXT), metadata (TEXT JSON), steps (TEXT JSON array).
This table is READ-ONLY. Do not issue INSERT, UPDATE, or DELETE against it.

## Brain Notes (this run)

[polish] [type:... tags:...] ` with attributes

Let me produce the final documentation:

---

# Brain Notes: Persistent Intelligence Across Runs

Brain notes are structured insights extracted from pipeline and agent step outputs. They persist in your store and are automatically injected into subsequent steps to build context over time.

## How Brain Blocks Work

When a store is configured (the default for `glitch pipeline run`), the system scans all step outputs for `<brain>` blocks. Any block found is automatically extracted, parsed, and stored—**the agent decides what's worth remembering, not the pipeline author.**

```yaml
steps:
  - id: audit
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Audit the codebase for security issues.
      Record your key findings in a <brain> block at the end.
```

When this step completes, any `<brain>` block in the output is detected and persisted automatically.

## Writing Brain Blocks

Use the `<brain>` tag with optional attributes to structure your insights:

```
<brain type="finding" tags="security,sql-injection" title="SQL injection in auth module">
Found parameterized queries missing in three locations:
- user_search.go line 42
- admin_filter.go line 89  
- report_query.go line 156

Root cause: string concatenation instead of prepared statements.
[rewrite] [type:research title:Audit codebase for security issues tags:security,sql-injection] Audit the codebase for security issues. Be specific.
Record your key findings in a <brain> block at the end.
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

Looking at the code and specifications provided, I have sufficient information to complete this task. The brain block system supports:

1. **Automatic scanning**: Brain blocks are detected in any step output when a store is configured
2. **Injection control**: Via `use_brain` YAML field (pipeline/step level) and `--brain` CLI flag
3. **Structured format**: `<brain type="..." tags="..." title="">` with attributes

Let me produce the final documentation:

---

# Brain Notes: Persistent Intelligence Across Runs

Brain notes are structured insights extracted from pipeline and agent step outputs. They persist in your store and are automatically injected into subsequent steps to build context over time.

## How Brain Blocks Work

When a store is configured (the default for `glitch pipeline run`), the system scans all step outputs for `<brain>` blocks. Any block found is automatically extracted, parsed, and stored—**the agent decides what's worth remembering, not the pipeline author.**

```yaml
steps:
  - id: audit
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Audit the codebase for security issues.
      Record your key findings in a <brain> block at the end.
```

When this step completes, any `<brain>` block in the output is detected and persisted automatically.

## Writing Brain Blocks

Use the `<brain>` tag with optional attributes to structure your insights:

```
<brain type="finding" tags="security,sql-injection" title="SQL injection in auth module">
Found parameterized queries missing in three locations:
- user_search.go line 42
- admin_filter.go line 89  
- report_query.go line 156

Root cause: string concatenation instead of prepared statements.
</brain>
```

### Attributes

| Attribute | Values | Purpose |
|-----------|--------|---------|
| `type` | `research`, `finding`, `data`, `code` | Categorize the note for filtering and reuse |
| `tags` | comma-separated | Add searchable metadata (e.g., `security,auth`) |
| `title` | string | Human-readable summary (recommended) |

## Controlling Brain Injection

Brain context is injected into subsequent steps by default when a local provider is active. Control this behavior three ways:

### 1. Pipeline-Level Control (YAML)

```yaml
use_brain: true
steps:
  - id: research
    executor: claude
    model: llama3.2
    prompt: "Research the pipeline setup"
  - id: improve
    executor: claude
    model: llama3.2
    prompt: "Improve based on findings"  # receives brain context
```

All steps inherit `use_brain: true` unless overridden.

### 2. Step-Level Override (YAML)

```yaml
use_brain: true
steps:
  - id: search
    executor: claude
    model: llama3.2
    prompt: "Find issues"
  - id: report
    use_brain: false  # override: suppress brain for this step
    executor: claude
    model: llama3.2
    prompt: "Write summary without context"
```

A step with `use_brain: false` suppresses brain injection even when the pipeline-level flag is `true`.

### 3. Command-Line Control (ask)

```bash
# Enable brain (default for local providers)
glitch ask --brain=true "explain my setup"

# Disable brain (useful for remote providers like claude)
glitch ask -p claude --brain=false "hello"

# Disable for all steps in a pipeline
glitch ask --pipeline my-task --brain=false "input data"
```

**Default behavior:**
- **Local providers** (ollama, shell): `--brain=true` (brain enabled)
- **Remote providers** (claude): `--brain=false` (brain disabled; opt in with `--brain=true`)

## Backward Compatibility

The legacy `<brain_notes>` tag is still recognized and stored identically to `<brain>`. Migrate to `<brain>` with attributes for better queryability.

```
<!-- Both are equivalent -->
<brain_notes>Finding: auth tokens stored unencrypted</brain_notes>
<brain type="finding" tags="auth,security">Auth tokens stored unencrypted</brain>
```

## Example: Multi-Step Pipeline with Brain

```yaml
use_brain: true
steps:
  - id: baseline-check
    executor: claude
    model: llama3.2
    prompt: |
      Audit the codebase for performance bottlenecks.
      Structure findings in a <brain type="finding"> block.

  - id: profiling
    executor: shell
    vars:
      args: "top -o %CPU -n 100"
    prompt: "Run profiling"

  - id: analysis
    executor: claude
    model: llama3.2
    prompt: |
      Using the baseline findings and profiling data,
      propose optimizations with implementation priority.
```

The `analysis` step automatically receives the brain context from `baseline-check` (findings, profiling data) prepended to its prompt.

## Querying Brain Notes

Access stored brain notes via the store API:

```go
s, _ := store.Open()
defer s.Close()

// Retrieve notes tagged with "security"
notes, _ := s.QueryBrainNotes(ctx, "tags:security")
for _, note := range notes {
    fmt.Printf("%s (%s)\n", note.Title, note.Type)
}
```

Exported as JSON in backups and queryable by `type` and `tags`.

