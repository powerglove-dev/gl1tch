## ADDED Requirements

### Requirement: APM agent capabilities persisted as brain notes on install
When an APM agent is installed via `InstallAndWrap()`, each string in the agent's `capabilities` frontmatter list SHALL be written to the brain store as a note with `run_id=0` (system-scope), tagged `type:capability source:apm title:<agent-name>`.

#### Scenario: agent with capabilities list installs successfully
- **WHEN** an `.agent.md` file has a non-empty `capabilities` list and `InstallAndWrap()` completes
- **THEN** one brain note per capability string SHALL be upserted to the store with `run_id=0` and tags `type:capability source:apm title:<agent-name>`

#### Scenario: agent with no capabilities list
- **WHEN** an `.agent.md` file has no `capabilities` frontmatter key
- **THEN** no capability brain notes SHALL be written and install proceeds normally

#### Scenario: re-install of same agent
- **WHEN** `InstallAndWrap()` runs for an agent that was previously installed
- **THEN** existing capability notes for that agent SHALL be upserted (not duplicated), keyed by `(title, source:apm)` composite

#### Scenario: capability notes visible to pipeline steps
- **WHEN** a pipeline step's context is assembled by `StoreBrainInjector`
- **THEN** system-scope capability notes with `source:apm` SHALL appear in the `## gl1tch Capabilities` section alongside `source:builtin` notes

### Requirement: CapabilitySeeder tags builtin notes with source
The `CapabilitySeeder` SHALL tag all notes it generates with `source:builtin` to distinguish them from APM-sourced capability notes.

#### Scenario: seeder runs after manager is populated
- **WHEN** `CapabilitiesFromManager` is called with a populated executor manager
- **THEN** all seeded notes SHALL include the tag `source:builtin`
