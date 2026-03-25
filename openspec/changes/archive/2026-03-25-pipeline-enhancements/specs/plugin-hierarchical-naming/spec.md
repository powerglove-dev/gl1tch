## ADDED Requirements

### Requirement: Plugin manager supports category.action lookup
`plugin.Manager` SHALL support resolving a step type in the format `<category>.<action>` (e.g. `providers.claude.chat`). The manager SHALL split the type on the last `.` to derive the category (`providers.claude`) and action (`chat`), look up the plugin registered under the category name, and pass `action` in the step vars map as `"_action"`. If no plugin is found under the category name, the manager SHALL fall back to looking up the full string as a plugin name.

#### Scenario: category.action resolves to registered plugin
- **WHEN** a plugin named `providers.claude` is registered and the step type is `providers.claude.chat`
- **THEN** the manager resolves the plugin to `providers.claude` and passes `_action: "chat"` in vars

#### Scenario: Full name fallback when no category match
- **WHEN** no plugin named `providers.claude` is registered and the step type is `providers.claude.chat`
- **THEN** the manager looks up `providers.claude.chat` as a direct plugin name

#### Scenario: Single-word type still resolves directly
- **WHEN** a step type is `"weather"` (no dot)
- **THEN** the manager resolves it directly as the plugin name `"weather"`

### Requirement: Builtin steps are checked before plugin lookup
The runner SHALL check the builtin step registry before querying the plugin manager. Step types prefixed with `builtin.` SHALL always resolve from the builtin registry and SHALL NOT delegate to the plugin manager.

#### Scenario: builtin.assert resolved from registry
- **WHEN** a step declares `type: builtin.assert`
- **THEN** the built-in assert executor is used, regardless of any plugin named `builtin.assert`

#### Scenario: Unknown builtin returns clear error
- **WHEN** a step declares `type: builtin.unknown`
- **THEN** the runner returns an error: `"unknown builtin step: builtin.unknown"`
