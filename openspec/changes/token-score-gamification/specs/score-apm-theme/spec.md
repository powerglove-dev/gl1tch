## ADDED Requirements

### Requirement: game-world pack carries two Ollama prompts
An APM game world pack SHALL be a `.agent.md` file with `kind: game-world` in its YAML frontmatter. It SHALL define two prompts used for the two-phase Ollama call:

- `game_rules`: system prompt for Phase 1 (evaluate). Tells Ollama how to interpret run data, what achievements exist and their conditions, ICE classification rules, and the expected JSON output format.
- `narrator_style`: system prompt for Phase 2 (narrate). Defines the voice, aesthetic, and formatting constraints for the narration text.

```yaml
kind: game-world
name: gl1tch-world-cyberspace
description: Hackers/Sneakers cyberpunk MUD aesthetic

game_rules: |
  You are the game engine for a text MUD called The Gibson.
  Given the player's run data, return only a JSON object:
    {"achievements": ["id", ...], "ice_class": "black-ice"|"trace-ice"|"data-ice"|null, "quest_events": []}

  Achievement definitions — evaluate based on run data:
  - ghost-runner: output tokens produced with high efficiency (output/input > 0.8)
  - cache-warlock: cache_read_tokens >= 10000 in this run
  - speed-demon: duration_ms < 2000 and output_tokens > 0
  - token-miser: efficiency ratio (output/input) > 0.5
  - cost-cutter: cost_usd == 0 and output_tokens > 0
  - first-blood: total_runs == 1 (check player's run count)
  - streak-3: streak_days >= 3

  ICE classification (only if step_failures > 0):
  - black-ice: non-zero exit
  - trace-ice: timeout
  - data-ice: output parse failure

  Return only valid JSON. No explanation.

narrator_style: |
  You are Zero Cool narrating a cyberpunk text MUD in the style of Hackers (1995).
  Terse. Confident. Slightly ominous. The Gibson speaks through you.
  3-6 lines. No markdown. No bullet points. No headers.
  Write the narration for this completed operation.
  Include: what happened, XP earned, level/rank, streak if > 1 day, achievements if any.
```

#### Scenario: Pack installs via apm
- **WHEN** a `.agent.md` with `kind: game-world` is installed via APM
- **THEN** it is recognized as a game world pack and used for subsequent runs

#### Scenario: Both prompts required for pack to be valid
- **WHEN** a pack is missing either `game_rules` or `narrator_style`
- **THEN** it is rejected at load time and the default embedded pack is used with a DEBUG log

### Requirement: GameWorldPack struct and loader interface
`internal/game` SHALL define:

```go
type GameWorldPack struct {
    Name          string
    GameRules     string  // system prompt for evaluate call
    NarratorStyle string  // system prompt for narrate call
}

type WorldPackLoader interface {
    ActivePack() GameWorldPack
}
```

`DefaultWorldPackLoader` SHALL return the embedded `gl1tch-world-cyberspace` pack. When an APM `kind: game-world` pack is installed, `APMWorldPackLoader` SHALL return it instead.

#### Scenario: No pack installed returns default
- **WHEN** no game-world pack is installed
- **THEN** `ActivePack()` returns the embedded Hackers/Sneakers pack

#### Scenario: Installed pack takes precedence
- **WHEN** one game-world pack is installed and valid
- **THEN** `ActivePack()` returns that pack

### Requirement: Pack authors can define any achievement set and any narrator voice
The `game_rules` prompt is free-form text — pack authors define achievements in natural language. Ollama interprets them. There is no schema validation on achievement conditions. A pack can define entirely different achievements, drop built-in ones, or add domain-specific ones (e.g., git-workflow achievements, language-specific milestones).

#### Scenario: Custom achievement defined in prose
- **WHEN** a pack defines "night-owl: run completed between 00:00 and 05:00 local time" in game_rules
- **THEN** Ollama evaluates the condition and may return "night-owl" in the achievements array

#### Scenario: Pack with completely different voice
- **WHEN** a pack defines narrator_style as a corporate satire voice instead of cyberpunk
- **THEN** the narration uses that voice with no engine changes required
