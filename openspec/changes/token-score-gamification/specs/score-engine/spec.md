## ADDED Requirements

### Requirement: XP is computed from token efficiency, cache utilization, and speed
The score engine SHALL compute XP from a `TokenUsage` and retry count using the following formula:

```
EfficiencyRatio = OutputTokens / (InputTokens + CacheCreationTokens)   // 0 if denominator is 0
BaseXP          = int64(OutputTokens * EfficiencyRatio * 10)
CacheBonus      = int64(CacheReadTokens / 2)
SpeedBonus      = max(0, 1000 - DurationMS/100)
PenaltyRetry    = RetryCount * 50
FinalXP         = max(0, BaseXP + CacheBonus + SpeedBonus - PenaltyRetry)
```

FinalXP SHALL never be negative.

#### Scenario: High-efficiency run earns XP
- **WHEN** `OutputTokens == 100`, `InputTokens == 200`, `CacheCreationTokens == 0`, `DurationMS == 5000`, `RetryCount == 0`
- **THEN** `EfficiencyRatio == 0.5`, `BaseXP == 500`, `FinalXP == 500 + 0 + 0 == 500`

#### Scenario: Cache hit adds bonus
- **WHEN** `CacheReadTokens == 10000`
- **THEN** `CacheBonus == 5000`

#### Scenario: Fast run adds speed bonus
- **WHEN** `DurationMS == 1000`
- **THEN** `SpeedBonus == 900`

#### Scenario: Retries penalize XP
- **WHEN** `RetryCount == 3`
- **THEN** penalty applied is 150 XP

#### Scenario: XP never goes negative
- **WHEN** penalties exceed BaseXP
- **THEN** `FinalXP == 0`

#### Scenario: Zero token run awards zero XP
- **WHEN** `TokenUsage{}` is passed
- **THEN** `FinalXP == 0`

### Requirement: Level table maps total XP to a level and title
The score engine SHALL provide a level table with at minimum the following entries. Level SHALL be the highest entry whose XP threshold is not exceeded by total XP:

```
Level  1:       0 XP — Apprentice of the Shell
Level  2:     500 XP — Journeyman Prompter
Level  3:    1500 XP — Adept of the Token
Level  4:    3000 XP — Conjurer of Context
Level  5:    5000 XP — Mage of Efficient Context
Level  6:    8000 XP — Sorcerer of the Stream
Level  7:   12000 XP — Warlock of the Window
Level  8:   15000 XP — Archon of Cache
Level  9:   22000 XP — Void Walker
Level 10:   30000 XP — Wizard of the Sparse Prompt
Level 12:   50000 XP — Elder of the Token
Level 15:   75000 XP — Grand Necromancer of Parsimony
Level 20:  150000 XP — The Tokenless One
```

#### Scenario: Level resolved from total XP
- **WHEN** total XP is 2000
- **THEN** level is 3, title is "Adept of the Token"

#### Scenario: Exact threshold is that level
- **WHEN** total XP is exactly 500
- **THEN** level is 2, title is "Journeyman Prompter"

#### Scenario: XP beyond max level stays at max level
- **WHEN** total XP exceeds 150000
- **THEN** level is 20, title is "The Tokenless One"

### Requirement: Achievement detection runs after every score event
The score engine SHALL evaluate all achievement conditions after each score event and return a slice of newly unlocked achievement IDs. An achievement SHALL only be returned once — the engine SHALL not re-emit achievements already recorded in the store.

Built-in achievements:

| ID | Name | Condition |
|----|------|-----------|
| `first-blood` | First Blood | total_runs == 1 |
| `cache-warlock` | Cache Warlock | cache_read_tokens >= 10000 in a single run |
| `the-minimalist` | The Minimalist | input_tokens < 500 and output_tokens > 0 |
| `speed-demon` | Speed Demon | duration_ms < 2000 and output_tokens > 0 |
| `streak-3` | On a Roll | streak_days >= 3 |
| `multi-provider` | Polyglot | 3 distinct providers used within a calendar day |
| `token-miser` | Token Miser | efficiency_ratio > 0.5 on a run |
| `cost-cutter` | Cost Cutter | cost_usd == 0 and output_tokens > 0 |

#### Scenario: First run unlocks first-blood
- **WHEN** score event recorded and total_runs becomes 1
- **THEN** achievement `first-blood` is returned

#### Scenario: Achievement not re-emitted
- **WHEN** `first-blood` is already in the store
- **THEN** it is NOT returned in the newly unlocked slice

### Requirement: Streak is updated daily based on run dates
The engine SHALL increment `streak_days` when a run occurs on a calendar day immediately following the previous run day (local time). If a day is skipped, `streak_days` SHALL reset to 1. If the run is on the same day as the last run, `streak_days` is unchanged.

#### Scenario: Consecutive days increment streak
- **WHEN** last_run_date is yesterday and a new run occurs today
- **THEN** streak_days increments by 1

#### Scenario: Skipped day resets streak
- **WHEN** last_run_date is two or more days ago
- **THEN** streak_days resets to 1

#### Scenario: Same day does not change streak
- **WHEN** last_run_date is today
- **THEN** streak_days is unchanged
