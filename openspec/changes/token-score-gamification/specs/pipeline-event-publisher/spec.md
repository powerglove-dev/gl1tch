## ADDED Requirements

### Requirement: game.run.scored topic published after run scoring completes
The pipeline runner SHALL publish a single `game.run.scored` event to the `EventPublisher` after each run that completes (success or failure). The topic constant SHALL be defined in `internal/busd/topics/topics.go` as `TopicGameRunScored = "game.run.scored"`.

The payload SHALL be the complete `GameRunScoredPayload` JSON struct (defined in `internal/game/`) containing token usage, XP breakdown, level delta, streak, achievements, and ICE encounters. The switchboard SHALL be able to render the full narration from this payload alone without additional store queries.

#### Scenario: game.run.scored published after pipeline run completes
- **WHEN** a pipeline run finishes (success or failure)
- **THEN** the EventPublisher receives one call with topic `"game.run.scored"` and a non-nil payload

#### Scenario: game.run.scored published even on zero token usage
- **WHEN** a run completes and token capture returns zero values
- **THEN** `game.run.scored` is still published with `xp == 0` and zero token fields

#### Scenario: Exactly one event per run, not per step
- **WHEN** a pipeline has three steps
- **THEN** exactly one `game.run.scored` event is published (after the final step), not three

### Requirement: ICE encounters accumulated per run and included in payload
For each step that fails (non-zero exit, timeout, or parse error), the runner SHALL record an ICE encounter struct and include all encounters in the `game.run.scored` payload.

```go
type ICEEncounter struct {
    StepID    string // step identifier
    ICEClass  string // "black-ice" | "trace-ice" | "data-ice"
    Retried   bool
    TokenCost int64  // tokens burned on failed attempt(s)
    Resolved  bool   // true if step eventually succeeded
}
```

#### Scenario: Failed step produces ICE encounter in payload
- **WHEN** a step exits with non-zero status
- **THEN** `ice_encounters` in the payload contains one entry with matching StepID

#### Scenario: Clean run has empty ice_encounters
- **WHEN** all steps succeed on first attempt
- **THEN** `ice_encounters` is an empty array in the payload
