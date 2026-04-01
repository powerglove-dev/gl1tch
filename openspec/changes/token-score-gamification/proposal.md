## Why

Every AI tool now exposes token usage in structured output, but gl1tch discards it. Token efficiency is a real skill — getting meaningful work done with fewer tokens costs less money and signals tighter prompting. Making that visible and rewarding it turns a hidden metric into a feedback loop that makes users better at working with AI.

The delivery mechanism is already in place: gl1tch speaks in the chat panel after every run. The game lives entirely there. No new panels, no HUD, no extra UI surface. Just gl1tch narrating what happened — in the voice of a 90s hacker movie — every time you do something.

## What Changes

- New `internal/game/` package: token capture, XP engine, ICE classification, narrator, world pack loader
- New SQLite tables (`score_events`, `user_score`, `achievements`) tracked alongside existing run records
- `CliAdapter` output tee'd through token capture parsers for Claude, Codex, Copilot, and Gemini
- BUSD bridge: `pipeline.*` and `agent.*` events translated to game events, then injected as gl1tch messages in the chat panel
- gl1tch speaks in MUD/Hackers voice after every run — narrating token efficiency, XP delta, level, streak, ICE encounters, achievements — all through the existing chat window
- APM `kind: game-world` packs control the narrator voice, level titles, zone names, ICE descriptions, and achievement flavor text — the engine stays fixed, the language is swappable

## Capabilities

### New Capabilities

- `token-capture`: Per-provider token usage extraction from CLI output streams (Claude, Codex, Copilot, Gemini)
- `score-engine`: XP calculation from token efficiency, level progression, streak tracking, achievement detection, ICE classification from failed steps
- `score-persistence`: SQLite schema and store methods for score events, user state, and achievements
- `game-narrator`: Formats game events as MUD-style narration strings and injects them as gl1tch assistant messages in the chat panel — the sole UI surface for the game
- `game-world-pack`: APM `kind: game-world` pack format — controls narrator voice, level titles, ICE descriptions, achievement flavor text; default `gl1tch-world-cyberspace` pack embedded as fallback

### Modified Capabilities

- `pipeline-event-publisher`: Emit `game.run.scored` event after run scoring completes

## Impact

- `internal/executor/cli_adapter.go`: Output tee'd through capture layer; no interface changes
- `internal/pipeline/runner.go`: Call game engine after each step/run completes
- `internal/store/schema.go`: Three new tables added via migration
- `internal/busd/topics/topics.go`: One new topic constant (`game.run.scored`)
- `internal/console/switchboard.go`: Subscribe to `game.run.scored`, inject narration into gl1tch chat panel via existing `ClarificationInjectMsg` path
- No new TUI panels, subcommands, left column, or status HUD
- No breaking changes to existing executor, pipeline, or brain interfaces
