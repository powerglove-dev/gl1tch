## 1. Store: brain_notes Schema and Methods

- [x] 1.1 Add `brain_notes` table DDL to `internal/store/schema.go` with columns: `id`, `run_id`, `step_id`, `created_at`, `tags`, `body`
- [x] 1.2 Add idempotent `applyBrainNotesTableMigration` function following the existing `applyStepsColumnMigration` pattern
- [x] 1.3 Call the new migration from `applySchema` in `store.go`
- [x] 1.4 Define `BrainNote` struct in `store` package (`RunID`, `StepID`, `CreatedAt`, `Tags`, `Body` fields)
- [x] 1.5 Implement `InsertBrainNote(ctx, BrainNote) (int64, error)` on `Store`
- [x] 1.6 Implement `RecentBrainNotes(ctx, runID int64, limit int) ([]BrainNote, error)` ordered by `created_at DESC`
- [x] 1.7 Write table-driven unit tests for `InsertBrainNote` and `RecentBrainNotes` covering: insert+retrieve round-trip, multi-run isolation, limit cap, empty-result case

## 2. Pipeline: BrainInjector Interface and Default Implementation

- [x] 2.1 Create `internal/pipeline/brain.go` defining the `BrainInjector` interface with `ReadContext(ctx context.Context, runID int64) (string, error)`
- [x] 2.2 Implement `StoreBrainInjector` struct that holds a `*store.Store` reference
- [x] 2.3 Implement `StoreBrainInjector.ReadContext`: fetch schema summary (hardcoded), call `RecentBrainNotes` capped at 10, truncate each note body at 500 chars, assemble preamble with `## Brain Notes (this run)` section header
- [x] 2.4 Write unit tests for `StoreBrainInjector.ReadContext` covering: no notes (schema-only preamble), with notes (header present), 15-note cap to 10, 800-char body truncation with marker
- [x] 2.5 Add `WithBrainInjector(BrainInjector) RunOption` to the pipeline run options in `runner.go`

## 3. Pipeline: Struct Extensions for use_brain and write_brain

- [x] 3.1 Add `UseBrain bool` field (`yaml:"use_brain"`) to `Pipeline` struct in `pipeline.go`
- [x] 3.2 Add `WriteBrain bool` field (`yaml:"write_brain"`) to `Pipeline` struct in `pipeline.go`
- [x] 3.3 Add `UseBrain *bool` field (`yaml:"use_brain"`) to `Step` struct (pointer for tri-state)
- [x] 3.4 Add `WriteBrain *bool` field (`yaml:"write_brain"`) to `Step` struct (pointer for tri-state)
- [x] 3.5 Add helper `stepUseBrain(pipeline *Pipeline, step *Step) bool` and `stepWriteBrain(pipeline *Pipeline, step *Step) bool` resolving the tri-state inheritance

## 4. ExecutionContext: Brain Injector Integration

- [x] 4.1 Add `injector BrainInjector` and `runID int64` fields to `ExecutionContext` struct in `context.go`
- [x] 4.2 Add `SetBrainInjector(injector BrainInjector, runID int64)` method on `ExecutionContext`
- [x] 4.3 Store the `BrainInjector` on `runConfig` (parallel to how `WithRunStore` stores a `*store.Store`) so the option is available at `ec` creation time; transfer it to `ec` via `SetBrainInjector` when `ec` is first constructed inside `runLegacy`/`runDAG` — the `RunOption` cannot call `ec.SetBrainInjector` directly since `ec` does not exist yet at option-application time
- [x] 4.4 Add `BrainInjector() BrainInjector` accessor on `ExecutionContext`
- [x] 4.5 Write unit tests for tri-state resolution: pipeline true + step nil = true, pipeline true + step false = false, pipeline false + step true = true

## 5. Runner: Brain Pre-Context Injection (Read Path)

- [x] 5.1 In `runner.go` prompt-building path, after interpolating the prompt string, call `stepUseBrain` to check if injection is active
- [x] 5.2 If active and `ec.BrainInjector() != nil`, call `ReadContext` and prepend preamble to the prompt string
- [x] 5.3 If `BrainInjector` is nil but `use_brain` is true on a step, emit a debug log and continue without injection (silent no-op)
- [x] 5.4 If `ReadContext` returns an error, log at debug level and continue with original prompt
- [x] 5.5 Apply the same brain read injection to the **legacy runner path** (`executePluginStep` in `runLegacy`): after interpolating `promptOrInput`, check `stepUseBrain` and prepend preamble if active — the DAG path injection in `resolveExecutor` alone will not cover legacy pipelines
- [x] 5.6 Write integration tests: preamble present when injector provided + use_brain, preamble absent when injector nil, preamble absent when use_brain false; include one test using a legacy-format pipeline (no `needs` fields) to verify the legacy path injects correctly

## 6. Runner: Brain Write Injection and Response Parsing

- [x] 6.1 After the read preamble (if any), check `stepWriteBrain`; if active, append the write-context instruction block to the prompt
- [x] 6.2 After step execution completes (output captured), check `stepWriteBrain`; if active, scan output string for `<brain ...>...</brain>` pattern using a simple regex
- [x] 6.3 If a brain block is found, extract `tags` attribute (empty string if absent) and body text; call `store.InsertBrainNote` via the store attached to `ExecutionContext`
- [x] 6.4 If brain block is found but XML is malformed, log debug message; do not fail the step
- [x] 6.5 If no brain block is found on a write_brain step, log a debug message; do not fail the step
- [x] 6.6 Write unit tests for brain block parsing: valid with tags, valid without tags, malformed (unclosed), no block present
- [x] 6.7 Apply the same write injection and `<brain>` response parsing to the **legacy runner path** (`executePluginStep` / `runLegacy`) — same gap as the read path; both paths must be updated
- [x] 6.8 Document the known limitation that `<brain>` XML blocks remain visible in the step's output string (as surfaced in the feed/inbox UI); add a TODO comment in the runner code noting that a future `strip_brain_output: true` pipeline flag could strip the block from user-visible output before it is published

## 7. Remove db Step Type

- [x] 7.1 Delete `internal/pipeline/step_db.go`
- [x] 7.2 Remove the `"db"` case from `resolveExecutor` in `runner.go`
- [x] 7.3 Add validation in `pipeline.Load` (or runner validation) that returns an error containing "db step type has been removed" if any step has `type: db`
- [x] 7.4 Write a test asserting that a pipeline with `type: db` returns an error at load/validation time
- [x] 7.5 Verify no `.pipeline.yaml` files in the repo use `type: db` (grep check)

## 8. End-to-End Integration Test

- [x] 8.1 Write an integration test with a mock plugin that records its input; assert the brain preamble appears in the prompt when `use_brain: true` and a `StoreBrainInjector` is provided
- [x] 8.2 Write an integration test where step A has `write_brain: true` and emits `<brain tags="t">insight</brain>` in output; assert `brain_notes` row is inserted with correct fields
- [x] 8.3 Write an integration test for the feedback loop: step A writes a brain note; step B (later in same pipeline, `use_brain: true`) receives that note in its preamble
- [x] 8.4 Write an integration test asserting that step B in a different pipeline run does NOT receive step A's notes (run isolation)

## 9. Pipeline YAML Validation and Documentation

- [ ] 9.1 Update pipeline YAML validation to accept `use_brain` and `write_brain` as valid top-level and step-level fields (no spurious "unknown field" warnings)
- [ ] 9.2 Add at least one example pipeline YAML in `testdata/` demonstrating `use_brain` + `write_brain` usage with comments
- [ ] 9.3 If a `.pipeline.yaml` validator or linter exists, update it to know about the new fields and the removed `db` type
