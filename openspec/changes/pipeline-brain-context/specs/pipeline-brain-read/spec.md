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
- **WHEN** a pipeline has `use_brain: false` (or no `use_brain` field) and a step has no `use_brain` field
- **THEN** no brain preamble is injected for that step

### Requirement: BrainInjector interface provides read context
The `pipeline` package SHALL define a `BrainInjector` interface with a `ReadContext(ctx context.Context, runID int64) (string, error)` method. The `StoreBrainInjector` default implementation SHALL query the store for the current run's schema summary and up to 10 most recent `brain_notes` rows. The assembled preamble SHALL be a plain-text block describing: (1) the `runs` table schema, (2) how to interpret run data, (3) recent brain notes, and (4) an explicit instruction that the agent should NOT attempt to modify data.

#### Scenario: ReadContext returns a non-empty preamble when brain notes exist
- **WHEN** the store has 3 `brain_notes` rows for the current run and `ReadContext` is called
- **THEN** the returned string contains the schema description and all 3 note bodies

#### Scenario: ReadContext returns schema-only preamble when no brain notes exist
- **WHEN** no `brain_notes` rows exist for the current run
- **THEN** `ReadContext` returns a non-empty string containing the schema description without a notes section

#### Scenario: ReadContext caps notes at 10 entries
- **WHEN** 15 `brain_notes` rows exist for the current run
- **THEN** `ReadContext` returns at most 10 note bodies in the preamble

#### Scenario: Individual note bodies are truncated at 500 characters
- **WHEN** a `brain_notes` row has a `body` field of 800 characters
- **THEN** the preamble includes at most 500 characters of that body, with a truncation marker

#### Scenario: Notes section uses the exact header Brain Notes (this run)
- **WHEN** at least one brain note exists for the current run
- **THEN** the preamble contains the header `## Brain Notes (this run)` immediately before the note bodies

#### Scenario: Notes section is omitted entirely when no notes exist
- **WHEN** no brain notes exist for the current run
- **THEN** the preamble does NOT contain any `## Brain Notes` heading

### Requirement: Runner prepends brain read context to agent step prompts
The `runner.go` `resolveExecutor` (or its prompt-building path) SHALL call `BrainInjector.ReadContext` when `use_brain` is active for a step and prepend the result to the interpolated `prompt` string before dispatching to the plugin.

#### Scenario: Brain preamble is prepended before plugin receives prompt
- **WHEN** a step with `use_brain: true` is executed and `BrainInjector.ReadContext` returns a non-empty string
- **THEN** the plugin's input string begins with the preamble text, followed by the original interpolated prompt

#### Scenario: Brain injection does not alter prompt when use_brain is inactive
- **WHEN** a step has `use_brain: false` or no `use_brain` flag and pipeline-level flag is also false/unset
- **THEN** the plugin receives the prompt exactly as it would without brain injection

#### Scenario: Runner continues without preamble if BrainInjector returns an error
- **WHEN** `BrainInjector.ReadContext` returns an error
- **THEN** the step proceeds with the original prompt; the error is logged at debug level; the step does not fail

### Requirement: db step type is removed
The `db` step type (backed by `step_db.go`) SHALL be removed from the pipeline executor registry. Any pipeline YAML referencing `type: db` SHALL fail with a clear error message at parse/load time.

#### Scenario: Pipeline with type db fails at load time
- **WHEN** a pipeline YAML contains a step with `type: db`
- **THEN** `pipeline.Load` (or runner validation) returns an error containing "db step type has been removed"

#### Scenario: Pipeline without type db loads normally
- **WHEN** a pipeline YAML contains no `type: db` steps
- **THEN** `pipeline.Load` succeeds
