## ADDED Requirements

### Requirement: apm.yml supports pipeline stanza per entry
Each entry in `apm.yml` MAY include a `pipeline` stanza containing a minimal pipeline YAML fragment that describes how to invoke the agent as a single pipeline step.

#### Scenario: pipeline stanza present and agent installs successfully
- **WHEN** `apm.yml` declares a `pipeline` stanza for an agent and `InstallAndWrap()` completes successfully
- **THEN** a pipeline template file SHALL be written to `~/.config/glitch/pipelines/apm.<name>.pipeline.yaml`

#### Scenario: pipeline stanza absent
- **WHEN** `apm.yml` declares an agent without a `pipeline` stanza
- **THEN** no pipeline template file SHALL be written and install proceeds normally

#### Scenario: pipeline template file already exists
- **WHEN** `InstallAndWrap()` runs and `~/.config/glitch/pipelines/apm.<name>.pipeline.yaml` already exists
- **THEN** the existing file SHALL NOT be overwritten and a notice SHALL be logged indicating the skip

#### Scenario: pipelines directory does not exist
- **WHEN** the pipelines config directory does not exist at materialization time
- **THEN** the directory SHALL be created before writing the template file
