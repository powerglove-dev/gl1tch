## ADDED Requirements

### Requirement: Store persists per-step records alongside each run
The store `runs` table SHALL include a `steps` column (TEXT, default `'[]'`) containing a JSON array of step records. Each step record SHALL include: `id` (string), `status` (string: waiting/running/done/failed/skipped), `started_at` (RFC3339 or empty), `finished_at` (RFC3339 or empty), `duration_ms` (int), and `output` (object, may be null).

#### Scenario: Step record written on step completion
- **WHEN** a pipeline step completes successfully
- **THEN** the store's `steps` column for that run contains a JSON entry with `status: "done"` and `duration_ms` set to a positive integer

#### Scenario: Step record written on step failure
- **WHEN** a pipeline step fails after all retry attempts
- **THEN** the store's `steps` column for that run contains a JSON entry with `status: "failed"` and `finished_at` set

#### Scenario: Parallel steps each produce independent step records
- **WHEN** a DAG pipeline runs two steps concurrently
- **THEN** the `steps` array contains two entries, each with their own `started_at` and `duration_ms`

#### Scenario: Legacy runs with no step data are valid
- **WHEN** a run was recorded before this change (empty `steps` column)
- **THEN** the store returns an empty slice for that run's steps and no error is raised

### Requirement: StoreWriter interface exposes RecordStepComplete
The `StoreWriter` interface SHALL include `RecordStepComplete(ctx context.Context, runID int64, step StepRecord) error`. Callers pass one `StepRecord` per step transition to done, failed, or skipped. Implementations SHALL upsert the step entry in the JSON array by `id`.

#### Scenario: RecordStepComplete upserts by step id
- **WHEN** `RecordStepComplete` is called twice for the same step id (e.g., status update from running to done)
- **THEN** the `steps` array contains exactly one entry for that id with the latest data

#### Scenario: RecordStepComplete is a no-op when run id is unknown
- **WHEN** `RecordStepComplete` is called with a run id that does not exist in the store
- **THEN** it returns an error and does not modify any other row

### Requirement: QueryRuns returns step records with each Run
The `store.Run` struct SHALL include a `Steps []StepRecord` field. `QueryRuns` SHALL populate this field by parsing the `steps` JSON column. An unparseable column SHALL return an empty slice (not an error) and log a warning.

#### Scenario: QueryRuns includes step data
- **WHEN** a run with two completed steps is queried via QueryRuns
- **THEN** the returned `Run.Steps` slice has length 2 and each entry has a non-empty `status`

#### Scenario: Empty steps column returns empty slice
- **WHEN** a run has `steps = '[]'` or NULL
- **THEN** `Run.Steps` is an empty slice and no error is returned
