## MODIFIED Requirements

### Requirement: ExecutionContext is a named type with path-based accessors
The pipeline package SHALL expose an `ExecutionContext` struct that wraps a `map[string]any` with a `sync.RWMutex` for concurrent access. It SHALL provide `Get(path string) (any, bool)` and `Set(path string, value any)` methods. `Get` SHALL support dot-separated path expressions (e.g. `"step.fetch.data.url"`) by walking nested maps. The struct SHALL additionally hold an optional `BrainInjector` reference and a `runID int64` field, set via `WithBrainInjector` and `WithRunStore` pipeline options respectively.

#### Scenario: Set and Get round-trip
- **WHEN** `ec.Set("param.env", "staging")` is called
- **THEN** `ec.Get("param.env")` returns `("staging", true)`

#### Scenario: Dot-path traversal
- **WHEN** the context contains `{"step": {"fetch": {"data": {"url": "https://x.com"}}}}`
- **THEN** `ec.Get("step.fetch.data.url")` returns `("https://x.com", true)`

#### Scenario: Missing path returns false
- **WHEN** `ec.Get("step.missing.data.key")` is called on a context without that path
- **THEN** it returns `(nil, false)`

#### Scenario: Snapshot returns a deep copy
- **WHEN** `ec.Snapshot()` is called
- **THEN** it returns a `map[string]any` that reflects the current state; subsequent `Set` calls do not modify the snapshot

#### Scenario: BrainInjector is accessible from context
- **WHEN** `WithBrainInjector(injector)` is passed to `pipeline.Run`
- **THEN** the runner can retrieve the injector from `ExecutionContext` and call `ReadContext`

## REMOVED Requirements

### Requirement: db step executor is registered in the runner
**Reason**: The `db` step type (`type: db` in pipeline YAML, backed by `step_db.go`) is removed. Its read use-case is superseded by `use_brain`; its write use-case is superseded by `write_brain`. The `dbExecutor` struct and its registration in `resolveExecutor` are deleted.
**Migration**: Remove any `type: db` steps from pipeline YAML files. Use `use_brain: true` on agent steps that need to reason about database contents. Use `write_brain: true` on agent steps that should persist insights.
