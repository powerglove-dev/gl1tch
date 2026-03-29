## ADDED Requirements

### Requirement: brain_notes table is added to the ORCAI SQLite store
The `store` package SHALL create a `brain_notes` table via schema migration on startup. The table SHALL have columns: `id INTEGER PRIMARY KEY AUTOINCREMENT`, `run_id INTEGER NOT NULL`, `step_id TEXT NOT NULL`, `created_at INTEGER NOT NULL`, `tags TEXT DEFAULT ''`, `body TEXT NOT NULL`. The `run_id` column SHALL reference the `runs.id` of the pipeline run that produced the note. The table SHALL be created if it does not exist (idempotent migration).

#### Scenario: brain_notes table created on fresh store
- **WHEN** `store.Open` is called on a database with no existing schema
- **THEN** the `brain_notes` table exists with all required columns

#### Scenario: brain_notes migration is idempotent
- **WHEN** `store.Open` is called on a database that already has the `brain_notes` table
- **THEN** no error is returned and the table is unchanged

### Requirement: Store exposes InsertBrainNote and RecentBrainNotes methods
The `store.Store` type SHALL expose `InsertBrainNote(ctx context.Context, note BrainNote) (int64, error)` and `RecentBrainNotes(ctx context.Context, runID int64, limit int) ([]BrainNote, error)`. `BrainNote` SHALL be a struct with fields `RunID int64`, `StepID string`, `CreatedAt int64`, `Tags string`, and `Body string`.

#### Scenario: InsertBrainNote persists a note and returns its ID
- **WHEN** `InsertBrainNote` is called with a valid `BrainNote`
- **THEN** it returns a non-zero ID and the note is retrievable via `RecentBrainNotes`

#### Scenario: RecentBrainNotes returns notes for the specified run in reverse chronological order
- **WHEN** 5 notes exist for run 42 and `RecentBrainNotes(ctx, 42, 10)` is called
- **THEN** 5 notes are returned, ordered by `created_at` descending

#### Scenario: RecentBrainNotes respects the limit parameter
- **WHEN** 15 notes exist for a run and limit is 10
- **THEN** exactly 10 notes are returned

#### Scenario: RecentBrainNotes returns empty slice when no notes exist
- **WHEN** no notes exist for the requested run_id
- **THEN** an empty slice and nil error are returned

### Requirement: Pipeline and Step structs support write_brain flag
The `Pipeline` struct SHALL include a `WriteBrain bool` field (`yaml:"write_brain"`). The `Step` struct SHALL include a `WriteBrain *bool` field (`yaml:"write_brain"`) using a pointer tri-state. Inheritance and override semantics SHALL be identical to `use_brain`.

#### Scenario: Pipeline-level write_brain activates for all agent steps
- **WHEN** a pipeline YAML sets `write_brain: true` and contains two agent steps with no step-level flag
- **THEN** both steps have the write-brain preamble injected and their responses are scanned for `<brain>` blocks

#### Scenario: Step-level write_brain false suppresses write injection
- **WHEN** a pipeline sets `write_brain: true` and one step sets `write_brain: false`
- **THEN** that step does not receive the write preamble and its response is not parsed for `<brain>` blocks

### Requirement: Runner injects write context preamble for write_brain steps
When a step has `write_brain` active, the runner SHALL append a write-context block to the prompt (after any read-context preamble from `use_brain`). The write-context block SHALL instruct the agent to: (1) include a `<brain>` XML element in its response, (2) use the format `<brain tags="optional,comma,tags">note body text</brain>`, and (3) understand that this note will be persisted and made available to future agents in this run.

#### Scenario: Write preamble appended after read preamble when both flags are active
- **WHEN** a step has both `use_brain: true` and `write_brain: true`
- **THEN** the plugin receives: [read preamble] + [original prompt] + [write instruction]

#### Scenario: Write preamble appended to unmodified prompt when only write_brain is active
- **WHEN** a step has `write_brain: true` and `use_brain` is not active
- **THEN** the plugin receives: [original prompt] + [write instruction]

### Requirement: Runner parses brain XML block from agent response and persists to store
After a `write_brain` step completes, the runner SHALL scan the plugin's output string for a `<brain ...>...</brain>` block. If found, it SHALL call `store.InsertBrainNote` with the extracted body and tags. The step result SHALL not be modified — the `<brain>` block remains in the output. If no `<brain>` block is found, the runner SHALL log a debug message and take no other action.

#### Scenario: Brain note persisted when agent response contains valid brain XML
- **WHEN** a write_brain step's plugin output contains `<brain tags="summary">some insight</brain>`
- **THEN** `InsertBrainNote` is called with `Body="some insight"`, `Tags="summary"`, and the current `RunID` and `StepID`

#### Scenario: No error when agent response contains no brain XML
- **WHEN** a write_brain step's plugin output contains no `<brain>` element
- **THEN** no note is inserted and the step completes successfully

#### Scenario: Brain XML with no tags attribute is accepted
- **WHEN** the agent emits `<brain>plain note with no tags</brain>`
- **THEN** the note is inserted with an empty `Tags` field

#### Scenario: Malformed brain XML does not fail the step
- **WHEN** the agent emits `<brain unclosed content`
- **THEN** the note is not inserted, a debug log is emitted, and the step succeeds normally
