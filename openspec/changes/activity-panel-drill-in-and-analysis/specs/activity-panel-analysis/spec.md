## ADDED Requirements

### Requirement: On-demand analysis SHALL use the OpenCode tool-using loop driven by qwen2.5:7b

All on-demand analysis of indexed documents invoked from the activity modal SHALL execute through the existing OpenCode-based `Analyzer` tool-using loop, with its model pinned to **qwen2.5:7b** via a local Ollama-compatible backend by default. No other model path is permitted for this feature, and the model selection SHALL be parameterized (not hardcoded in Go source) so it can be changed via configuration.

#### Scenario: Analysis invocation uses qwen2.5:7b via OpenCode

- **WHEN** the modal triggers any analysis (all, selected, or follow-up refinement)
- **THEN** the backend invokes the `Analyzer` with model `qwen2.5:7b` via the configured local Ollama backend, and the tool-using loop has access to its standard tool set (filesystem, shell, ES query, git)

#### Scenario: Operator overrides the default model via configuration

- **WHEN** configuration sets a different local model for the activity analyzer
- **THEN** subsequent analysis invocations use the configured model instead of `qwen2.5:7b` without any code change

### Requirement: Analysis classification and next-step generation SHALL be LLM-driven

The system SHALL NOT use Go-side regular expressions, keyword lists, static severity tables, or other pattern-matching heuristics to classify, rank, score, or decide the importance of indexed documents. All such judgment SHALL be delegated to the LLM via a prompt. Editable prompt rubrics MAY exist as prompt files but MUST NOT encode fixed taxonomies that the system relies on in code.

#### Scenario: New error class appears in indexed documents

- **WHEN** indexed documents contain an error shape the system has never seen before
- **THEN** the LLM analyzer is given the raw documents and the user's prompt and produces its own classification and next steps without any Go code adding to a hardcoded error list

#### Scenario: Prompt rubric is edited

- **WHEN** an operator edits a prompt file under `pkg/glitchd/prompts/`
- **THEN** the next analysis run uses the edited prompt without recompiling the binary

### Requirement: Modal SHALL support "Analyze all" and "Analyze selected" with optional user prompt

The analysis pane of the drill-in modal SHALL expose two invocation controls: "Analyze all" (acts over every document in the current source/window view) and "Analyze selected (N)" (acts only over the currently checked documents). Both SHALL accept an optional free-form user prompt. When the user prompt is empty, the system SHALL still invoke the analyzer using only the default rubric.

#### Scenario: User runs "Analyze selected" with a prompt

- **WHEN** the user selects 3 documents, types "focus on test failures", and clicks "Analyze selected"
- **THEN** the backend fetches those 3 full `Event` records, constructs an analyzer input combining them with the user prompt, and invokes the tool-using loop

#### Scenario: User runs "Analyze all" with no prompt

- **WHEN** the user clicks "Analyze all" with an empty prompt box
- **THEN** the backend constructs analyzer input over the full document set with only the default rubric and invokes the loop

### Requirement: Analysis output SHALL stream into the modal in real time

The analyzer SHALL stream tokens back to the frontend as they are produced so the user can see progress. The stream SHALL be delivered via a dedicated Wails event keyed by a per-invocation stream id. On completion, a terminal marker SHALL be emitted so the frontend can transition the UI to the "refinement" state.

#### Scenario: Tokens stream while the analyzer runs

- **WHEN** the analyzer produces output tokens
- **THEN** the frontend receives them via `brain:analysis:stream` keyed by the invocation's `streamID` and renders them as markdown in the analysis pane

#### Scenario: Analysis completes

- **WHEN** the analyzer finishes its final turn
- **THEN** a terminal event is emitted on `brain:analysis:stream` for the `streamID`, and the modal reveals the follow-up prompt box

#### Scenario: Analysis errors mid-stream

- **WHEN** the analyzer fails mid-run
- **THEN** the terminal event carries an error field, the modal surfaces the error inline, and the partial output so far remains visible

### Requirement: Follow-up refinement SHALL chain analyses via parent_analysis_id

After the first analysis completes, the modal SHALL present a follow-up prompt box. Each refinement invocation SHALL be a new analyzer run whose input includes the referenced document set (or subset) and the new user prompt, and whose persisted output SHALL carry a `parent_analysis_id` linking it to the previous analysis in the chain.

#### Scenario: User refines a completed analysis

- **WHEN** the user types a follow-up prompt and submits it after an analysis completes
- **THEN** a new analyzer run is invoked, its result is persisted with `parent_analysis_id` set to the previous analysis's id, and both analyses are reachable from the activity sidebar as a linked chain

#### Scenario: Multi-turn refinement

- **WHEN** the user runs three sequential refinements on the same base analysis
- **THEN** each persisted analysis's `parent_analysis_id` points at its immediate predecessor, forming an ordered chain

### Requirement: Every analysis run SHALL be persisted and surfaced as an activity event

On completion, every successful analysis run SHALL be indexed into the `glitch-analyses` Elasticsearch index with a deterministic `event_key` derived from its inputs, and SHALL be emitted as a `brain:activity` event of kind `analysis` with `parent_id` referencing the triggering indexing or alert event.

#### Scenario: Analysis completes and is persisted

- **WHEN** an analysis run finishes successfully
- **THEN** its markdown output and metadata are indexed into `glitch-analyses` with `event_key = hash(source, eventIDs, userPrompt)` and a matching `kind: "analysis"` activity row is emitted

#### Scenario: Duplicate analysis inputs

- **WHEN** an analysis is invoked with inputs whose hash matches an already-indexed run
- **THEN** the system either returns the cached result or overwrites the existing document at the same `event_key` — in either case no duplicate activity row is produced
