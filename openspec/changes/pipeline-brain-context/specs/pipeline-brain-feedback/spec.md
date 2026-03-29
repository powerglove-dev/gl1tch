## ADDED Requirements

### Requirement: Brain notes written in a pipeline run are surfaced to subsequent use_brain steps in the same run
When a pipeline has both `use_brain` and `write_brain` active (at any combination of pipeline or step level), the `BrainInjector.ReadContext` implementation SHALL include any `brain_notes` rows already written during the current pipeline run in the preamble delivered to later steps. Notes are ordered newest-first. This creates a within-run feedback loop: each agent step can build on insights recorded by earlier steps.

#### Scenario: Note written by step A appears in preamble of step B
- **WHEN** step A (with `write_brain: true`) runs first and inserts a brain note, and step B (with `use_brain: true`) runs after
- **THEN** step B's prompt preamble includes the body of the note written by step A

#### Scenario: Note from a different run is not included
- **WHEN** a brain note with a different `run_id` exists in the store
- **THEN** that note does NOT appear in the preamble of any step in the current run

#### Scenario: Steps without use_brain do not receive brain notes even if write_brain wrote notes earlier
- **WHEN** a previous step wrote a brain note and a later step has `use_brain: false`
- **THEN** the later step's prompt does not include any brain note content

### Requirement: BrainInjector preamble identifies notes written in the current run
The read-context preamble assembled by `StoreBrainInjector` SHALL distinguish between notes written in the current run and notes from previous runs (when `brain_scope: all` is set, future). For Phase 1, only current-run notes are included and the preamble header for the notes section SHALL read: `## Brain Notes (this run)`.

#### Scenario: Notes section header is present when notes exist
- **WHEN** the current run has at least one brain note
- **THEN** the preamble contains the header `## Brain Notes (this run)` followed by the note bodies

#### Scenario: Notes section is omitted when no notes exist
- **WHEN** no brain notes have been written for the current run
- **THEN** the preamble does not contain a `## Brain Notes` section

### Requirement: Pipeline run ID is passed to BrainInjector via ExecutionContext
The pipeline `Run` function SHALL accept a `WithBrainInjector(injector BrainInjector)` option. When provided, the injector SHALL be stored on the `ExecutionContext` and the current `run_id` (from the store if available, else 0) SHALL be set on the context so `BrainInjector.ReadContext` can scope queries correctly.

#### Scenario: Run with WithBrainInjector stores injector on context
- **WHEN** `pipeline.Run` is called with `WithBrainInjector(injector)` and `WithRunStore(store)`
- **THEN** the runner uses the provided injector for all brain-active steps

#### Scenario: Run without WithBrainInjector performs no brain injection even if use_brain flag is set
- **WHEN** `pipeline.Run` is called without `WithBrainInjector` but a step has `use_brain: true`
- **THEN** no preamble is injected and no error is returned (silent no-op with a debug log)

### Requirement: Brain feedback improves pipeline prompt quality over iterative runs
Brain notes written to `brain_notes` SHALL be queryable by future pipeline runs when `brain_scope: all` is added (Phase 2). For Phase 1, only same-run notes are in scope, but the schema and injector architecture SHALL be designed to support cross-run surfacing without structural changes.

#### Scenario: brain_notes table retains notes across runs
- **WHEN** a pipeline run inserts brain notes and completes
- **THEN** those notes remain in the `brain_notes` table and are queryable by run_id after the run ends

#### Scenario: Future run can query historical notes by run_id
- **WHEN** a query is made with `RecentBrainNotes(ctx, oldRunID, 10)` after that run has ended
- **THEN** the notes from that run are returned correctly
