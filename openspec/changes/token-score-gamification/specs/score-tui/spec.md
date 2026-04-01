## ADDED Requirements

### Requirement: All game output is delivered as async gl1tch assistant messages in the chat panel
After a run completes and `game.run.scored` is published to BUSD, the switchboard SHALL fire a goroutine and return immediately. The goroutine calls Ollama, waits for the response, then injects the narration as a gl1tch assistant message via the existing `ClarificationInjectMsg` path. The run and all callers are never blocked.

No new TUI panels, widgets, subcommands, or status bar additions are introduced. The chat panel is the sole game UI surface.

#### Scenario: Narration appears in chat panel after run
- **WHEN** a pipeline run completes and `game.run.scored` is published
- **THEN** the gl1tch chat panel displays the narration as an assistant message (after Ollama responds)

#### Scenario: BUSD subscriber returns immediately
- **WHEN** the switchboard receives `game.run.scored`
- **THEN** the subscriber returns without waiting for Ollama, and a goroutine handles the rest

#### Scenario: Ollama unavailable does not surface an error
- **WHEN** the Ollama call fails or times out
- **THEN** no message is injected, no error is shown to the user, failure is logged at DEBUG

### Requirement: Two-phase Ollama call — evaluate then narrate
The game goroutine SHALL make two sequential Ollama calls:

**Phase 1 — Evaluate** (structured output):
- Input: run data JSON + pack's `game_rules` prompt
- Output: `{"achievements": ["id1", ...], "ice_class": "black-ice" | null, "quest_events": []}`
- Result persisted to store before Phase 2 begins
- If JSON is malformed, retry once with stricter prompt; on second failure skip evaluation for this run

**Phase 2 — Narrate** (free-form text):
- Input: run data JSON + evaluation result + pack's `narrator_style` prompt
- Output: plain text narration string (3–6 lines, no markdown)
- Injected into chat panel via `ClarificationInjectMsg`

#### Scenario: Evaluate result persisted before narration injected
- **WHEN** Phase 1 completes with valid JSON
- **THEN** achievements are written to store before the narrate call begins

#### Scenario: Narrate call receives evaluation result as context
- **WHEN** Phase 2 runs
- **THEN** the Ollama prompt includes the evaluation output (achievements unlocked, ICE class) so narration reflects them

#### Scenario: Malformed evaluate JSON retried once
- **WHEN** Phase 1 returns non-JSON output
- **THEN** one retry is made; on second failure evaluation is skipped and narration proceeds with empty evaluation context

### Requirement: Narrator voice and game rules come from the active world pack
The game goroutine SHALL load the active `GameWorldPack` before making Ollama calls. The pack provides `game_rules` (system prompt for Phase 1) and `narrator_style` (system prompt for Phase 2). When no APM `kind: game-world` pack is installed, the embedded default Hackers/Sneakers pack is used.

#### Scenario: Default pack used when none installed
- **WHEN** no APM world pack is installed
- **THEN** the embedded default prompts are used

#### Scenario: Installed pack overrides default
- **WHEN** one `kind: game-world` pack is installed
- **THEN** its `game_rules` and `narrator_style` prompts are used instead of the default

### Requirement: Run data context passed to both Ollama calls
Both calls SHALL receive a JSON context object built from the scored run:

```json
{
  "provider": "claude",
  "model": "claude-sonnet-4-6",
  "input_tokens": 2,
  "output_tokens": 5,
  "cache_read_tokens": 19018,
  "cache_creation_tokens": 0,
  "cost_usd": 0.07,
  "duration_ms": 2961,
  "xp": 127,
  "xp_breakdown": {"base": 0, "cache_bonus": 95, "speed_bonus": 32, "retry_penalty": 0},
  "total_xp": 2847,
  "level": 4,
  "level_title": "Adept of the Token",
  "prev_level": 3,
  "next_level_xp": 5000,
  "streak_days": 3,
  "step_failures": 0
}
```

#### Scenario: Context includes level delta
- **WHEN** a run causes a level increase
- **THEN** `prev_level` is less than `level` in the context passed to Ollama

### Requirement: game.run.scored BUSD event carries the complete scored result
The `game.run.scored` payload SHALL be a JSON object sufficient for the game goroutine to build the Ollama context without additional store queries at dispatch time:

```json
{
  "xp": 127,
  "xp_breakdown": {"base": 0, "cache_bonus": 95, "speed_bonus": 32, "retry_penalty": 0},
  "total_xp": 2847,
  "level": 4,
  "level_title": "Adept of the Token",
  "prev_level": 3,
  "next_level_xp": 5000,
  "streak_days": 3,
  "step_failures": 0,
  "usage": {
    "provider": "claude",
    "model": "claude-sonnet-4-6",
    "input_tokens": 2,
    "output_tokens": 5,
    "cache_read_tokens": 19018,
    "cache_creation_tokens": 0,
    "cost_usd": 0.07,
    "duration_ms": 2961
  }
}
```

#### Scenario: Payload deserializes without additional store queries
- **WHEN** the switchboard receives `game.run.scored`
- **THEN** the game goroutine can build its Ollama context from the payload alone
