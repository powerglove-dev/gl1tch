## ADDED Requirements

### Requirement: Brain loop SHALL run an LLM judge pass over new document batches

After each collector refresh that produces at least one new document, the brain loop SHALL invoke a lightweight LLM judge pass using **qwen2.5:7b** (no tools) over the new batch. The judge's role is to decide whether the batch is noteworthy enough to raise an alert to the user, assign a severity, produce a one-line hook, and return the referenced document ids. The judge SHALL NOT be given the ability to make changes or call tools — it is a read-only classifier.

#### Scenario: Judge sees a noteworthy batch

- **WHEN** a collector refresh produces a batch of new documents the judge classifies as noteworthy
- **THEN** the judge returns a non-none response carrying severity, one-line hook, and the referenced document ids

#### Scenario: Judge sees an unremarkable batch

- **WHEN** a collector refresh produces a batch the judge does not consider noteworthy
- **THEN** the judge returns `none` and no alert is raised

#### Scenario: Judge fails or times out

- **WHEN** the judge invocation errors or exceeds its timeout
- **THEN** the brain loop logs the failure, emits no alert for that batch, and continues with the next refresh cycle without retrying the same batch

### Requirement: Alert detection SHALL contain no Go-side heuristics

The implementation of the judge pass SHALL NOT include any Go-side regular expressions, keyword lists, static severity-by-source maps, or other pattern-matching heuristics that determine noteworthiness, severity, or hook text. All judgment SHALL flow through the LLM prompt. The Go code's sole responsibilities are assembling input, invoking the model, parsing the structured response, and wiring it into the activity emission path.

#### Scenario: New source type is added

- **WHEN** a new collector source is wired into the brain loop
- **THEN** alert detection works for that source without any change to judge-related Go code, because the LLM receives the raw docs and decides on its own

#### Scenario: Prompt file is edited

- **WHEN** an operator edits the judge prompt file under `pkg/glitchd/prompts/`
- **THEN** the next refresh cycle uses the edited prompt without a recompile

### Requirement: Noteworthy judge output SHALL emit an alert activity event

When the judge returns a non-none response, the brain SHALL emit a `brain:activity` event of kind `alert` carrying the judge's severity, hook, and the referenced document ids (`event_refs`), and referencing the triggering indexing event via `parent_id`.

#### Scenario: Alert emitted with referenced docs

- **WHEN** the judge returns a noteworthy response for a batch
- **THEN** a `kind: "alert"` activity event is emitted whose `title` is the judge's one-line hook, whose `severity` is the judge's severity, whose `parent_id` is the triggering indexing event id, and whose `event_refs` list contains the ids the judge flagged

#### Scenario: User clicks an alert row

- **WHEN** the user clicks an alert row in the activity sidebar
- **THEN** the drill-in modal opens pre-filtered to the alert's `event_refs` and the alert's hook is pre-filled into the modal's user prompt field

### Requirement: Judge pass SHALL be rate-limited to avoid duplicate work

The brain loop SHALL skip the judge pass when the new-doc delta is below a configured minimum, or when the hash of the current batch matches a recently judged batch. The goal is to avoid running the judge on every 30-second poll when nothing interesting is happening.

#### Scenario: Below-threshold delta

- **WHEN** a refresh produces fewer than the configured minimum new documents
- **THEN** the judge pass is skipped and no alert can be raised for that refresh

#### Scenario: Duplicate batch hash

- **WHEN** a refresh produces a batch whose content hash matches a recently judged batch
- **THEN** the judge pass is skipped and no alert is emitted
