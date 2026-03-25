## ADDED Requirements

### Requirement: Builtin steps show executor-specific arg fields in the Advanced group
When a `builtin.*` executor is selected, the Advanced group SHALL render a set of arg fields specific to that builtin type, pre-keyed with the expected arg names, rather than a blank key/value list. Unknown builtins fall back to the generic args editor.

#### Scenario: builtin.assert shows value and condition arg fields
- **WHEN** the executor is set to `builtin.assert`
- **THEN** the Advanced group shows pre-populated arg rows for `value` and `condition` with inline hints (e.g., `always`, `contains:<str>`, `matches:<regex>`, `len > <n>`)

#### Scenario: builtin.log shows message arg field
- **WHEN** the executor is set to `builtin.log`
- **THEN** the Advanced group shows a single pre-populated arg row for `message`

#### Scenario: builtin.sleep shows duration arg field
- **WHEN** the executor is set to `builtin.sleep`
- **THEN** the Advanced group shows a single pre-populated arg row for `duration` with hint `e.g. 2s, 500ms`

#### Scenario: builtin.http_get shows url arg field
- **WHEN** the executor is set to `builtin.http_get`
- **THEN** the Advanced group shows a pre-populated arg row for `url`

#### Scenario: builtin.set_data shows generic args editor
- **WHEN** the executor is set to `builtin.set_data`
- **THEN** the Advanced group shows the generic key/value args editor (set_data accepts arbitrary key/value pairs)

### Requirement: Builtin step type is indicated in the executor dropdown with a description hint
Each builtin entry in the executor dropdown SHALL include a short parenthetical description rendered in dim text next to the name, so the user understands the step type without needing external documentation.

#### Scenario: Builtin descriptions shown in dropdown
- **WHEN** the executor dropdown is open and the user scrolls to the builtins section
- **THEN** each builtin is shown as `builtin.log  (write message to output)` with the description in dim text

### Requirement: Saving a builtin step writes executor and args to YAML, not plugin and vars
When a step with a builtin executor is saved, the output YAML SHALL use `executor: builtin.<type>` and `args:` with the configured key/value pairs. The deprecated `plugin:` and `vars:` fields SHALL NOT be written for builtin steps.

#### Scenario: builtin.log step saved correctly
- **WHEN** the user configures a step with executor `builtin.log` and message arg `hello world`, then saves
- **THEN** the pipeline YAML contains `executor: builtin.log` and `args: {message: hello world}` for that step, with no `plugin:` or `vars:` keys
