## ~~1. SidecarSchema: system_prompt_source field~~ [DROPPED]

> This group was removed. The sidecar `system_prompt_source` approach was abandoned in favor of APM as the canonical skill path. Tasks 1.1–1.7 are not implemented.

## 2. APM Capability Brain Seeding

- [x] 2.1 Add `BrainStore` dependency to `ApmManager` (or pass via a seeder interface) in `internal/apmmanager/model.go`
- [x] 2.2 After successful `InstallAndWrap()`, iterate the parsed agent's `capabilities` slice
- [x] 2.3 For each capability, upsert a brain note with `run_id=0`, tags `type:capability source:apm title:<agent-name>`, body = capability string; use upsert-by-composite-key to avoid duplicates
- [x] 2.4 Update `CapabilitiesFromManager` in `internal/pipeline/capability.go` to tag seeded notes with `source:builtin`
- [x] 2.5 Write tests: install with capabilities list seeds correct notes; re-install upserts not duplicates; install without capabilities is a no-op; `source:builtin` tag present on seeder output

## 3. APM Skill Pipeline Materialization

- [x] 3.1 Define `pipeline` stanza struct in the `apm.yml` entry model (e.g., `ApmEntry.Pipeline string` — raw YAML fragment)
- [x] 3.2 Parse `pipeline` stanza when loading `apm.yml` entries in `ApmManager`
- [x] 3.3 In `InstallAndWrap()`, if `Pipeline` is non-empty, resolve output path `~/.config/glitch/pipelines/apm.<name>.pipeline.yaml`
- [x] 3.4 Write pipeline template file only if it does not already exist; log notice if skipping due to existing file
- [x] 3.5 Create pipelines config directory if it does not exist before writing
- [x] 3.6 Write tests: pipeline stanza present writes file; file already exists is skipped with notice; no stanza writes nothing; directory creation works

## 4. Router APM Awareness

- [x] 4.1 After `buildFullManager()` in `cmd/ask.go`, collect all executors with ID prefix `apm.`
- [x] 4.2 For each, synthesize description string `[apm] <name>: <description>` and inject into router embedding cache alongside pipeline YAML descriptions
- [x] 4.3 In router match result handling, detect when the matched ID is an `apm.` executor
- [x] 4.4 If the `apm.` executor is not registered (not yet installed), call `RequireAgent` to install it before dispatch
- [x] 4.5 On confident APM match (≥ 0.85), construct and dispatch a synthetic single-step pipeline targeting the `apm.<name>` executor
- [x] 4.6 If no APM executors are registered, router cache population is unchanged
- [x] 4.7 Write tests: APM executor descriptions appear in cache; confident match dispatches correctly; below-threshold falls through to LLM path; uninstalled agent triggers RequireAgent

## 5. Integration & Validation

- [x] 5.1 Add an integration test: install a test APM agent with capabilities, verify brain notes are seeded with correct tags
- [x] 5.2 Add an integration test: install a test APM agent with pipeline stanza, verify template file is created at correct path
- ~~5.3 Add a sidecar YAML fixture with `system_prompt_source: apm:<id>` and verify registration succeeds when agent is installed~~ [N/A — sidecar approach dropped]
- [x] 5.4 Smoke test router with a mock APM executor description and confirm embedding + dispatch path
- [x] 5.5 Update `apm.yml` in the repo with a `pipeline` stanza for at least one existing agent as a real-world example
