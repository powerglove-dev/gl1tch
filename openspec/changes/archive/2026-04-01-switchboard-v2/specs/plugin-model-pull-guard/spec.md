## ADDED Requirements

### Requirement: Ollama plugin checks model presence before pulling
The `orcai-ollama` plugin SHALL call `ollama list` at startup, parse the output to determine which models are already present locally, and skip `ollama pull` for the requested model if it is already available. The pull SHALL only be attempted if the model is not found in the list.

#### Scenario: Model already present — pull skipped
- **WHEN** the requested model appears in the output of `ollama list`
- **THEN** no `ollama pull` command is executed
- **AND** the plugin proceeds directly to inference

#### Scenario: Model not present — pull executed
- **WHEN** the requested model does not appear in the output of `ollama list`
- **THEN** the plugin executes `ollama pull <model>` before inference

#### Scenario: ollama list fails — pull attempted as fallback
- **WHEN** `ollama list` exits with a non-zero status or cannot be executed
- **THEN** the plugin falls back to the existing reactive pull behaviour (pull on 404)

### Requirement: Opencode plugin checks Ollama model presence before pulling
The `orcai-opencode` plugin's `pullOllamaModel` function SHALL call `ollama list` to check whether the requested model (with the `ollama/` prefix stripped) is already present before executing `ollama pull`. If the model is present, the function SHALL return nil immediately without pulling.

#### Scenario: Ollama model already present — pull skipped in opencode plugin
- **WHEN** the model (after stripping `ollama/` prefix) appears in `ollama list` output
- **THEN** `pullOllamaModel` returns nil without executing `ollama pull`

#### Scenario: Ollama model absent — pull executed in opencode plugin
- **WHEN** the model does not appear in `ollama list` output
- **THEN** `pullOllamaModel` executes `ollama pull <model>` and returns its error if any

#### Scenario: Non-ollama model skips check entirely
- **WHEN** the model string does not begin with `ollama/`
- **THEN** neither the list check nor the pull is executed
