## ADDED Requirements

### Requirement: Step executor has init / execute / cleanup lifecycle
Each pipeline step executor SHALL implement three lifecycle phases: `Init` (called once before execution to allocate resources), `Execute` (main execution, returns structured output), and `Cleanup` (called once after execution regardless of success or failure). The runner SHALL call these phases in order for every step.

#### Scenario: Cleanup called on success
- **WHEN** a step executes successfully
- **THEN** `Cleanup` is called after `Execute` returns

#### Scenario: Cleanup called on failure
- **WHEN** a step's `Execute` returns an error
- **THEN** `Cleanup` is still called before the step is marked failed

#### Scenario: Init failure skips execute
- **WHEN** a step's `Init` returns an error
- **THEN** `Execute` is NOT called, `Cleanup` IS called, and the step is marked failed

### Requirement: Steps return structured output as map[string]any
Each step executor's `Execute` method SHALL return `(map[string]any, error)`. The runner SHALL store the returned map at `step.<id>.data` in the execution context. A nil return SHALL be treated as an empty map (no output).

#### Scenario: Step output accessible by subsequent steps
- **WHEN** step `fetch` returns `{"url": "https://example.com"}`
- **THEN** a subsequent step template `{{step.fetch.data.url}}` resolves to `"https://example.com"`

#### Scenario: Nil output does not error
- **WHEN** a step returns nil output and no error
- **THEN** the step is marked done and `step.<id>.data` is an empty map

### Requirement: Template interpolation supports nested step output paths
The pipeline template interpolator SHALL support dot-separated path expressions for nested context access. A template string `{{step.<id>.data.<key>}}` SHALL walk the execution context map and substitute the value. If the path does not resolve, the placeholder SHALL be left unchanged and a warning logged.

#### Scenario: Nested path resolved
- **WHEN** the context contains `step.create.data.cluster_id = "abc123"`
- **THEN** a template `{{step.create.data.cluster_id}}` interpolates to `"abc123"`

#### Scenario: Missing path left unchanged
- **WHEN** a template references `{{step.missing.data.key}}` and no such step exists
- **THEN** the placeholder is left in the output string and a warning is logged

#### Scenario: Legacy {{stepID.out}} still works
- **WHEN** a pipeline uses the old `{{stepFetch.out}}` convention
- **THEN** the interpolator resolves it from `step.Fetch.data.value` (case-insensitive) and logs a deprecation warning
