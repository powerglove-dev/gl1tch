## Requirements

### Requirement: ExecutionContext is a named type with path-based accessors
The pipeline package SHALL expose an `ExecutionContext` struct that wraps a `map[string]any` with a `sync.RWMutex` for concurrent access. It SHALL provide `Get(path string) (any, bool)` and `Set(path string, value any)` methods. `Get` SHALL support dot-separated path expressions (e.g. `"step.fetch.data.url"`) by walking nested maps.

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

### Requirement: executeStep is a standalone function extracted from the runner loop
The pipeline package SHALL expose an internal `executeStep(ctx context.Context, step *Step, ec *ExecutionContext, mgr *plugin.Manager, w io.Writer) error` function. The existing `Run` loop SHALL call this function. The function's behaviour SHALL be identical to the current inline loop body.

#### Scenario: executeStep executes plugin and updates context
- **WHEN** `executeStep` is called with a valid plugin step
- **THEN** the plugin is invoked, output is captured, and `ec.Set(step.ID+".out", output)` is called

#### Scenario: executeStep returns error on plugin failure
- **WHEN** the plugin returns an error
- **THEN** `executeStep` returns that error without updating the context

### Requirement: template.Interpolate accepts map[string]any
`Interpolate(tmpl string, vars map[string]any) string` SHALL replace all `{{key}}` and `{{dot.path}}` placeholders by resolving the key or path against `vars`. Values of any type SHALL be coerced to string via `fmt.Sprint`. Unresolved placeholders SHALL be left unchanged.

#### Scenario: String value interpolated
- **WHEN** `vars = map[string]any{"name": "world"}` and `tmpl = "hello {{name}}"`
- **THEN** the result is `"hello world"`

#### Scenario: Nested path interpolated
- **WHEN** `vars = map[string]any{"step": map[string]any{"a": map[string]any{"out": "42"}}}` and `tmpl = "{{step.a.out}}"`
- **THEN** the result is `"42"`

#### Scenario: Unresolved placeholder left unchanged
- **WHEN** `vars` does not contain the key referenced in the template
- **THEN** the placeholder `{{missing}}` is left verbatim in the output

### Requirement: condition.EvalCondition accepts map[string]any
`EvalCondition(expr string, vars map[string]any) bool` SHALL accept a `map[string]any` vars argument. When an expression references a variable value, string/int/float/bool values SHALL be coerced to string for comparison via `fmt.Sprint`. Behaviour for all existing expression forms (`"always"`, `"contains:<s>"`, `"matches:<re>"`, `"len > <n>"`) SHALL be unchanged.

#### Scenario: Always expression returns true
- **WHEN** `EvalCondition("always", nil)` is called
- **THEN** it returns true

#### Scenario: contains expression with string-coerced value
- **WHEN** `vars = map[string]any{"count": 42}` and expr references count
- **THEN** comparison is performed against `"42"`
