## ADDED Requirements

### Requirement: Step editor groups fields into Core, Execution, and Advanced panels
The step editor right pane SHALL organize all step fields into three labeled groups: **Core** (executor, model, prompt), **Execution** (needs, retry max_attempts, retry interval, retry on, for_each, on_failure), and **Advanced** (condition if/then/else, publish_to, args). The active group SHALL be visually highlighted; other groups SHALL be rendered dim. `Tab` cycles forward through groups; `Shift+Tab` cycles backward.

#### Scenario: Tab advances to next group
- **WHEN** the user presses `Tab` while in the Core group
- **THEN** focus moves to the Execution group and all Execution fields are rendered at full brightness

#### Scenario: Shift+Tab retreats to previous group
- **WHEN** the user presses `Shift+Tab` while in the Execution group
- **THEN** focus moves to the Core group

#### Scenario: Inactive groups are rendered dim
- **WHEN** the Core group is active
- **THEN** the Execution and Advanced group labels and fields are rendered in the dim color (`\x1b[38;5;66m`)

### Requirement: Executor field uses a dropdown populated from installed providers and builtins
The executor field in the Core group SHALL be a Dropdown component populated with all installed providers (from `picker.BuildProviders()`) plus all builtin step types (`builtin.assert`, `builtin.log`, `builtin.sleep`, `builtin.http_get`, `builtin.set_data`), separated by a visual divider. Selecting an executor SHALL update the step's `Executor` field and clear `Model` if switching between provider and builtin categories.

#### Scenario: Provider executors appear above builtins
- **WHEN** the executor dropdown is opened
- **THEN** installed provider names appear first, followed by a separator, followed by builtin names

#### Scenario: Selecting a builtin clears the model field
- **WHEN** the user selects a `builtin.*` executor
- **THEN** the model field is cleared and shown as disabled (dim, not focusable)

#### Scenario: Selecting a provider enables the model field
- **WHEN** the user selects a provider executor (e.g., `claude`)
- **THEN** the model dropdown is populated with that provider's models and becomes focusable

### Requirement: Model field uses a dropdown populated from the selected executor's models
The model field SHALL be a Dropdown populated with the model list for the currently selected provider executor. The field SHALL be disabled (dim, non-focusable) when a builtin executor is selected.

#### Scenario: Model dropdown reflects selected provider
- **WHEN** the executor is set to `claude`
- **THEN** the model dropdown contains Claude model entries (e.g., Opus 4.6, Sonnet 4.6)

#### Scenario: Disabled model field is skipped by Tab
- **WHEN** the executor is a builtin and the user Tabs through Core fields
- **THEN** the model field is skipped and focus moves directly from executor to prompt

### Requirement: Needs field uses a multi-select dropdown listing sibling step IDs
The `needs` field in the Execution group SHALL be a multi-select dropdown showing all other step IDs in the pipeline. Selected IDs are shown with a `✓` prefix. `Space` toggles selection; `Enter` confirms.

#### Scenario: Multi-select allows multiple IDs to be checked
- **WHEN** the needs dropdown is open and the user presses `Space` on two different step IDs
- **THEN** both IDs are checked and shown with `✓`

#### Scenario: Confirmed selection is stored as the step's Needs slice
- **WHEN** the user presses `Enter` to close the needs dropdown
- **THEN** the step's `Needs` field contains exactly the checked IDs

### Requirement: Args field is an editable key/value list
The `args` field in the Advanced group SHALL render as a scrollable list of `key = value` rows. `+` adds a new blank row. `d` deletes the focused row. `Enter` on a row opens it for inline editing (key then value, Tab to advance).

#### Scenario: Adding a new args row
- **WHEN** the args list is focused and the user presses `+`
- **THEN** a new blank `key = value` row appears at the bottom of the list and is immediately in edit mode

#### Scenario: Deleting an args row
- **WHEN** a row in the args list is focused and the user presses `d`
- **THEN** the row is removed from the list and the step's `Args` map is updated

### Requirement: Step list pane reflects all steps with type indicators
The left pane step list SHALL display each step as `[N] id (executor)` where N is the 1-based index. The currently selected step SHALL be highlighted in pink. Steps with a builtin executor SHALL show a `⚙` prefix; provider steps show a `◆` prefix.

#### Scenario: Selected step highlighted in left pane
- **WHEN** the user navigates to step 2
- **THEN** step 2's row in the left pane is rendered in pink and step 1's row is dim

#### Scenario: Builtin step shows gear icon
- **WHEN** a step has executor `builtin.log`
- **THEN** the left pane shows `⚙ [2] step-id (builtin.log)`
