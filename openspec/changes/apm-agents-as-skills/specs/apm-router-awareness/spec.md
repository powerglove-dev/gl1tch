## ADDED Requirements

### Requirement: Router embedding cache includes APM agent descriptions
After `buildFullManager()` populates the executor manager, any executor with an ID prefix of `apm.` SHALL have its description injected into the router's embedding cache as a synthetic pipeline description string of the form `[apm] <name>: <description>`.

#### Scenario: APM executor present in manager at startup
- **WHEN** the executor manager contains one or more executors with IDs prefixed `apm.`
- **THEN** each SHALL be embedded and added to the router cache before the first user prompt is processed

#### Scenario: No APM executors registered
- **WHEN** no executors with the `apm.` prefix are present
- **THEN** the router cache SHALL be populated from pipeline YAML only, with no change in behavior

### Requirement: Confident APM router match triggers lazy install and dispatch
When the router resolves a user prompt to an APM agent description with cosine similarity ≥ 0.85 and the agent is not yet installed, the router SHALL trigger installation via `RequireAgent` before dispatching.

#### Scenario: matching APM agent already installed
- **WHEN** the router matches a prompt to an `apm.<name>` executor with confidence ≥ 0.85 and the executor is registered
- **THEN** a synthetic single-step pipeline SHALL be dispatched to that executor immediately

#### Scenario: matching APM agent not yet installed
- **WHEN** the router matches a prompt to an APM agent description but the executor is not yet registered
- **THEN** `RequireAgent` SHALL be called to install the agent, and dispatch SHALL proceed once installation completes

#### Scenario: APM match below confidence threshold
- **WHEN** the highest-similarity APM agent description scores below 0.85
- **THEN** the router SHALL fall through to the LLM classification path as normal
