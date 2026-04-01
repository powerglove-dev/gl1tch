## ADDED Requirements

### Requirement: Integration tests verify full pipeline execution
A suite of integration tests tagged `//go:build integration` SHALL exercise end-to-end pipeline execution by calling `pipeline.Load` and `pipeline.Run` with real pipeline YAML fixtures. Tests SHALL use the `llama3.2` and `qwen2.5` models via the ollama provider. Tests SHALL assert that `pipeline.Run` returns nil error and that at least one output line is produced.

#### Scenario: Full pipeline runs to completion with llama3.2
- **WHEN** an integration test loads and runs a single-step pipeline using `executor: ollama` and `model: llama3.2`
- **THEN** `pipeline.Run` returns nil
- **AND** the publisher receives at least one non-empty output line

#### Scenario: Full pipeline runs to completion with qwen2.5
- **WHEN** an integration test loads and runs a single-step pipeline using `executor: ollama` and `model: qwen2.5`
- **THEN** `pipeline.Run` returns nil
- **AND** the publisher receives at least one non-empty output line

#### Scenario: Integration test skips when model not available
- **WHEN** the required model is not present in `ollama list`
- **THEN** the test calls `t.Skip` with a message indicating the missing model rather than failing

### Requirement: Integration tests verify single-step agent pipeline (Quick Run)
Integration tests SHALL verify the Quick Run / agent flow by constructing an in-memory single-step `pipeline.Pipeline` (mirroring what the Switchboard's Quick Run builds) and running it via `pipeline.Run`. Tests SHALL use both `llama3.2` and `qwen2.5` models and assert successful output.

#### Scenario: Single-step agent pipeline with llama3.2
- **WHEN** an integration test builds a one-step pipeline with `executor: ollama`, `model: llama3.2`, and a short prompt
- **THEN** `pipeline.Run` returns nil
- **AND** at least one output line is received

#### Scenario: Single-step agent pipeline with qwen2.5
- **WHEN** an integration test builds a one-step pipeline with `executor: ollama`, `model: qwen2.5`, and a short prompt
- **THEN** `pipeline.Run` returns nil
- **AND** at least one output line is received

### Requirement: Integration tests are excluded from standard test runs
Integration tests SHALL use the Go build tag `integration` so that running `go test ./...` without the tag does not execute them. Running `go test -tags integration ./...` SHALL include them.

#### Scenario: Standard test run excludes integration tests
- **WHEN** `go test ./...` is executed without `-tags integration`
- **THEN** no integration test functions are compiled or run

#### Scenario: Tagged test run includes integration tests
- **WHEN** `go test -tags integration ./...` is executed
- **THEN** integration test functions are compiled and run
