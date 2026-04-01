## 1. Schema and Persistence

- [x] 1.1 Add `score_events`, `user_score`, and `achievements` tables to `internal/store/schema.go` via migration
- [x] 1.2 Add `ScoreEvent`, `UserScore`, and `ProviderScore` structs to `internal/store/store.go`
- [x] 1.3 Implement `RecordScoreEvent`, `GetUserScore`, `UpdateUserScore` store methods
- [x] 1.4 Implement `RecordAchievement`, `GetUnlockedAchievements` store methods
- [x] 1.5 Implement `ScoreEventsByProvider` store method
- [x] 1.6 Write table-driven tests for all store methods against an in-memory SQLite DB

## 2. Token Capture

- [x] 2.1 Define `TokenUsage` struct in `internal/game/capture.go`
- [x] 2.2 Implement `GameTeeWriter` — wraps `io.Writer`, accumulates output, routes to parser on `Close()`
- [x] 2.3 Implement Claude parser: scan for `"type":"result"` JSON, extract `usage` and `modelUsage` fields
- [x] 2.4 Implement Codex parser: scan JSONL for `"type":"turn.completed"`, extract `usage` fields
- [x] 2.5 Implement Copilot parser: accumulate `outputTokens` from `assistant.message` events, extract `totalApiDurationMs` from `result` event
- [x] 2.6 Implement Gemini parser: parse JSON for `prompt_token_count`, `candidates_token_count`, `cached_content_token_count`
- [x] 2.7 Register parsers in `map[string]TokenParser` keyed by executor category string
- [x] 2.8 Write unit tests for each parser using real captured output samples

## 3. XP Engine (deterministic Go)

- [x] 3.1 Implement `ComputeXP(usage TokenUsage, retryCount int) XPResult` in `internal/game/engine.go`
- [x] 3.2 Define `XPResult` struct: `Base`, `CacheBonus`, `SpeedBonus`, `RetryPenalty`, `Final`
- [x] 3.3 Implement built-in level table as sorted `[]LevelEntry{XP, Title}` slice
- [x] 3.4 Implement `LevelForXP(totalXP int64) (level int, title string, nextLevelXP int64)`
- [x] 3.5 Implement streak update: `UpdateStreak(state UserScore, now time.Time) UserScore`
- [x] 3.6 Write table-driven tests for XP formula, level resolution, and streak logic

## 4. Runner Integration

- [x] 4.1 Add `TopicGameRunScored = "game.run.scored"` to `internal/busd/topics/topics.go`
- [x] 4.2 In `internal/pipeline/runner.go`, wrap each step writer with `GameTeeWriter` when store is available
- [x] 4.3 Track step failure count during run execution
- [x] 4.4 After run completes: compute XP, update `user_score`, assemble `GameRunScoredPayload`
- [x] 4.5 Check game suppression before publishing: skip `game.run.scored` if pipeline frontmatter has `game: false` or global config has `game.enabled: false`
- [x] 4.6 Add `Game *bool` field to pipeline struct for the frontmatter flag; add `game.enabled` bool to global config struct
- [x] 4.7 Publish single `game.run.scored` BUSD event with marshalled payload (only when not suppressed)

## 5. World Pack and Ollama Game Engine

- [x] 5.1 Define `GameWorldPack` struct and `WorldPackLoader` interface in `internal/game/pack.go`
- [x] 5.2 Embed default `gl1tch-world-cyberspace` pack (game_rules + narrator_style prompts) as Go embed
- [x] 5.3 Implement `DefaultWorldPackLoader` returning the embedded pack
- [x] 5.4 Implement `APMWorldPackLoader` that reads installed `kind: game-world` agent from APM agent directory
- [x] 5.5 Implement `GameEngine.Evaluate(ctx, runData, pack) EvaluateResult` — Ollama call with `game_rules` as system prompt, structured JSON output
- [x] 5.6 Implement retry logic: if evaluate returns malformed JSON, retry once with stricter prompt; on second failure return empty evaluation result
- [x] 5.7 Persist evaluation result: call `RecordAchievement` for each newly unlocked achievement ID
- [x] 5.8 Implement `GameEngine.Narrate(ctx, runData, evalResult, pack) string` — Ollama call with `narrator_style` as system prompt, free-form text output
- [x] 5.9 Write tests for evaluate retry logic and graceful failure paths

## 6. Chat Injection

- [x] 6.1 In `internal/console/switchboard.go`, subscribe to `game.run.scored` BUSD topic
- [x] 6.2 On event received, fire goroutine immediately and return (non-blocking)
- [x] 6.3 Goroutine: load active world pack, call `GameEngine.Evaluate`, persist results, call `GameEngine.Narrate`
- [x] 6.4 Inject narration string as gl1tch assistant message via existing `ClarificationInjectMsg` path
- [x] 6.5 On any Ollama failure, log at DEBUG and exit goroutine silently — no user-visible error
