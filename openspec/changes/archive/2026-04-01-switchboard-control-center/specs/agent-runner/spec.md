## ADDED Requirements

### Requirement: Quick Run creates a single-step in-memory pipeline
The Quick Run section SHALL construct an in-memory `pipeline.Pipeline` with exactly one step when the user submits a prompt. The step SHALL use `executor: <providerID>`, `model: <modelID>` (if selected), and the prompt as the step's input. This pipeline SHALL be passed to `pipeline.Run` — the same function used for saved YAML pipelines. There SHALL be no separate execution code path for agent runs.

#### Scenario: Submit creates single-step pipeline and runs it
- **WHEN** the user completes provider + model selection and enters a prompt and presses Enter
- **THEN** an in-memory pipeline with one step is constructed and passed to `pipeline.Run`

#### Scenario: Quick Run and saved pipeline use same execution path
- **WHEN** either a Quick Run or a saved pipeline is launched
- **THEN** both call `pipeline.Run` with a `pipeline.Pipeline` struct; no separate CliAdapter.Execute path exists in the switchboard

### Requirement: Quick Run provides provider and model selection
The left column's Quick Run section SHALL present an inline three-step form:
1. Provider list (populated from `picker.BuildProviders()`, sidecar-aware).
2. Model list (populated from the selected provider's `Models`; skip if no models).
3. Prompt text input.

Tab or `→` advances between steps; `←` or `Esc` returns to the previous step.

#### Scenario: Provider list populated on switchboard open
- **WHEN** the switchboard initializes
- **THEN** the Quick Run section shows all providers returned by `picker.BuildProviders()`

#### Scenario: Model list populated after provider selection
- **WHEN** the user selects a provider with models and advances
- **THEN** the model list shows that provider's non-separator model entries

#### Scenario: Provider with no models skips model step
- **WHEN** the user selects a provider with an empty Models list and presses Tab
- **THEN** the form skips directly to the prompt input

#### Scenario: Tab advances form steps
- **WHEN** the user is on step N and presses Tab
- **THEN** focus moves to step N+1

### Requirement: Quick Run submits on Enter at prompt step
Pressing `Enter` while focused on the prompt input SHALL build the in-memory single-step pipeline and launch it via the same `launchPipelineCmd` used for saved pipelines. Output SHALL stream to the Activity Feed. Only one job runs at a time.

#### Scenario: Enter at prompt step launches single-step pipeline
- **WHEN** the user completes all form steps and presses Enter
- **THEN** a single-step pipeline runs and its output appears in the activity feed

#### Scenario: Activity feed shows provider and model in entry title
- **WHEN** a Quick Run job is running
- **THEN** the activity feed entry title shows `<providerID>/<modelID>` (or just `<providerID>` if no model)

### Requirement: Quick Run form clears after submission
After submission the prompt input SHALL clear and the form SHALL return to step 0 (provider selection).

#### Scenario: Form resets after submission
- **WHEN** the user submits and the job starts
- **THEN** the prompt clears and provider selection is refocused

### Requirement: Providers refresh on r key
Pressing `r` SHALL re-call `picker.BuildProviders()` and update the provider and model lists.

#### Scenario: r key refreshes provider list
- **WHEN** the user presses `r`
- **THEN** the provider list updates to include any newly installed sidecar plugins
