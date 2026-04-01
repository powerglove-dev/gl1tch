## Context

gl1tch wraps AI CLI tools (Claude, Codex, Copilot, Gemini) via `CliAdapter` in `internal/executor/cli_adapter.go`. Each adapter streams output to an `io.Writer` and returns when the subprocess exits. The pipeline runner in `internal/pipeline/runner.go` calls `executor.Execute()` per step and records results in SQLite via `internal/store/`. BUSD (`internal/busd/`) provides in-process pub/sub between the runner and the console TUI.

Ollama is a hard infrastructure requirement. It is already used for routing (embedding similarity, LLM classification) and brain scoring (OllamaSectionScorer). The assistant backend already streams Ollama responses token-by-token into the chat panel asynchronously.

The game lives entirely in the chat window. No new TUI panels. gl1tch speaks after every run via an async Ollama call — the run is never blocked, narration appears when Ollama is ready.

## Goals / Non-Goals

**Goals:**
- Capture token usage from all four supported providers without changing the executor interface
- Compute XP (deterministic arithmetic in Go) and persist to SQLite
- After each run, fire a non-blocking goroutine: call local Ollama with run data + APM pack prompts, inject the response into the gl1tch chat panel
- APM `kind: game-world` packs carry two prompts: `game_rules` (Ollama evaluates achievements, ICE class, quest events) and `narrator_style` (Ollama writes the narration text)
- Embedded default pack ships with the Hackers/Sneakers voice so the game works with no APM pack installed

**Non-Goals:**
- New TUI panels, subcommands, left column HUD, or status bar additions
- Blocking the run or the caller on Ollama response time
- Building a YAML rule interpreter in Go — Ollama is the rule engine
- Multi-user leaderboards or remote score sync
- Modifying the executor interface or pipeline YAML schema

## Decisions

### Ollama is the game engine, not a Go rule interpreter

**Decision:** Achievement evaluation, ICE classification, quest progress reasoning, and narration are all delegated to a local Ollama call. The APM pack's `game_rules` prompt tells Ollama the rules; the `narrator_style` prompt tells it the voice. Go only handles what must be deterministic: token arithmetic, XP math, and persistence.

**Rationale:** Writing a Go DSL interpreter for game rules (condition parsing, evaluation, edge cases) is significant complexity that buys nothing over a local LLM. Ollama is already running, already used for routing and scoring, and is a better rule engine — it reasons about novel situations, handles natural language conditions, and lets pack authors write rules in prose instead of a query language. The game design lives in the APM repo, not in Go.

**Alternative considered:** YAML condition DSL (`"efficiency_ratio > 0.8 AND output_tokens > 0"`). Rejected — requires a parser, an evaluator, type coercion, and error handling. Replaced entirely by Ollama.

### Non-blocking async narration via goroutine

**Decision:** The `game.run.scored` BUSD subscriber fires a goroutine immediately and returns. The goroutine makes the Ollama call, waits for the response, then injects the narration into the chat panel via the existing `ClarificationInjectMsg` path. The run and the caller are never blocked.

**Rationale:** Ollama response time is 1–3 seconds on a fast local model. Blocking the run on this is unacceptable. Async injection is already the established pattern — the assistant backend streams tokens into the chat panel the same way. The user is doing something else; the narration appears when it's ready.

### Two-phase Ollama call: evaluate then narrate

**Decision:** Two sequential Ollama calls per run, both using the pack's prompts:
1. **Evaluate** — structured JSON output: `{"achievements": [], "ice_class": null, "quest_events": []}`. Results persisted to store.
2. **Narrate** — free-form text output: the MUD narration string. Injected into chat.

Both calls receive the same run data context. The evaluate call uses a small/fast model (configurable, default `llama3.2`). The narrate call can use the same or a more expressive model.

**Rationale:** Separating evaluation (structured, persisted) from narration (creative, ephemeral) keeps the store clean and testable. If narration fails or Ollama is slow, the score data is already written. The two calls can also use different models — evaluation benefits from instruction-following accuracy, narration benefits from creative fluency.

**Alternative considered:** Single call returning both JSON decisions and narration. Rejected — mixing structured and free-form output in one response is brittle to parse.

### Tee output buffer, don't hook the executor interface

**Decision:** Wrap the `io.Writer` passed to `Execute()` with a `GameTeeWriter` that accumulates bytes and runs provider-specific parsers on close.

**Rationale:** Adding a token return value to `Executor.Execute()` would break every existing implementation and caller. A tee writer is invisible to the executor, composes cleanly with the existing step writer chain, and parses once on close.

### SQLite migration, additive only

**Decision:** Add `score_events`, `user_score`, and `achievements` tables via the existing schema migration system.

**Rationale:** Existing migration pattern. Single-row `user_score` enforced by `CHECK (id = 1)`. No config files — game state belongs with run history.

## APM Pack Format

```yaml
kind: game-world
name: gl1tch-world-cyberspace

game_rules: |
  You are the game engine for a text MUD called The Gibson.
  Given the player's run data, return a JSON object:
    {"achievements": ["id1"], "ice_class": "black-ice" | "trace-ice" | "data-ice" | null, "quest_events": []}

  Achievement definitions:
  - ghost-runner: output tokens produced with high efficiency (output/input > 0.8)
  - cache-warlock: heavy cache utilization (cache_read_tokens >= 10000)
  - speed-demon: completed in under 2000ms with output produced
  - token-miser: efficiency ratio > 0.5
  - cost-cutter: zero cost run (all cached)

  ICE class rules:
  - black-ice: step exited non-zero
  - trace-ice: step timed out
  - data-ice: output parse failure

  Return only valid JSON. No explanation.

narrator_style: |
  You are Zero Cool narrating a cyberpunk text MUD in the style of Hackers (1995).
  Terse. Confident. Slightly ominous. The Gibson speaks through you.
  3-6 lines. No markdown. No bullet points.
  Write the narration for this run.
```

The embedded default pack hardcodes this content. APM-installed packs override it entirely.

## Risks / Trade-offs

- **Ollama unavailable** → Game engine call fails silently. Score is persisted, no narration injected. Logged at DEBUG. The run is unaffected.
- **Ollama returns malformed JSON on evaluate call** → Retry once with stricter prompt. If still malformed, skip achievement evaluation for this run, persist zero achievements.
- **Provider output format drift** → Token parsers silently return zero usage. Logged at DEBUG. Zero-usage runs produce minimal narration ("Operation complete. No token data captured.").
- **Narration latency** → 1–3 seconds, fully async, never blocks the run. User is doing something else; message appears when ready.

## Migration Plan

1. Schema migration runs on next `glitch` startup — additive tables only
2. No existing pipelines, wrappers, or config files change
3. Score starts at zero — no backfill
4. Rollback: remove `internal/game/` package, drop three tables, remove BUSD subscription in switchboard

## Game Mode Suppression

Game narration SHALL be suppressible at two levels:

**Pipeline-level** — frontmatter flag in `.pipeline.yaml`:
```yaml
name: my-pipeline
game: false   # disables game narration for this pipeline
steps:
  ...
```

**Global** — config flag in `~/.config/glitch/config.yaml`:
```yaml
game:
  enabled: false
```

When either is set, the runner SHALL NOT publish `game.run.scored` at all. No goroutine is spawned, no Ollama call is made. Useful for CI, scripted runs, and any context where the chat panel isn't being watched.

The pipeline flag takes precedence for that run. The global flag is the off switch for all game activity.

## Open Questions

- Which Ollama model to use by default for evaluate vs. narrate calls? Suggest `llama3.2` for both, configurable in `~/.config/glitch/config.yaml` under `game.model`.
