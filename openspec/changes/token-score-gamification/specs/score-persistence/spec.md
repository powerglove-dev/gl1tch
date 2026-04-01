## ADDED Requirements

### Requirement: score_events table records each scored run
The store SHALL add a `score_events` table via the existing schema migration:

```sql
CREATE TABLE IF NOT EXISTS score_events (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id               INTEGER,
    xp                   INTEGER NOT NULL DEFAULT 0,
    input_tokens         INTEGER NOT NULL DEFAULT 0,
    output_tokens        INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens    INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd             REAL NOT NULL DEFAULT 0,
    provider             TEXT NOT NULL DEFAULT '',
    model                TEXT NOT NULL DEFAULT '',
    created_at           INTEGER NOT NULL
);
```

`run_id` references the `runs` table but is not a foreign key constraint (runs may not always be present for assistant-mode runs).

#### Scenario: Score event recorded after pipeline run
- **WHEN** a pipeline step completes with token data and XP is computed
- **THEN** a row is inserted into `score_events` with matching run_id and token counts

#### Scenario: Zero-XP runs are still recorded
- **WHEN** a run completes with zero output tokens
- **THEN** a score event row is inserted with xp == 0

### Requirement: user_score table holds cumulative user state as a single row
The store SHALL add a `user_score` table:

```sql
CREATE TABLE IF NOT EXISTS user_score (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    total_xp      INTEGER NOT NULL DEFAULT 0,
    level         INTEGER NOT NULL DEFAULT 1,
    streak_days   INTEGER NOT NULL DEFAULT 0,
    last_run_date TEXT NOT NULL DEFAULT '',
    total_runs    INTEGER NOT NULL DEFAULT 0
);
```

On first use the store SHALL insert the initial row `(1, 0, 1, 0, '', 0)` if it does not exist. Updates SHALL use `INSERT OR REPLACE`.

#### Scenario: Initial row created on first access
- **WHEN** `GetUserScore()` is called on a fresh database
- **THEN** a row is returned with all zero/default values and no error

#### Scenario: XP accumulates across runs
- **WHEN** two score events of 100 XP each are recorded
- **THEN** `GetUserScore()` returns `total_xp == 200`

### Requirement: Store exposes methods for score read and write
The `store.Store` SHALL expose:

```go
RecordScoreEvent(ctx context.Context, e ScoreEvent) error
GetUserScore(ctx context.Context) (UserScore, error)
UpdateUserScore(ctx context.Context, s UserScore) error
GetUnlockedAchievements(ctx context.Context) ([]string, error)
RecordAchievement(ctx context.Context, achievementID string) error
ScoreEventsByProvider(ctx context.Context) (map[string]ProviderScore, error)
```

`ProviderScore` SHALL hold total XP, total runs, and total tokens for a given provider.

#### Scenario: GetUnlockedAchievements returns empty slice on fresh DB
- **WHEN** no achievements have been recorded
- **THEN** returns an empty (not nil) slice

#### Scenario: ScoreEventsByProvider groups by provider field
- **WHEN** two score events exist with provider "claude" and one with "codex"
- **THEN** map has two keys with correct totals per provider

### Requirement: achievements table records unlocked achievement IDs
```sql
CREATE TABLE IF NOT EXISTS achievements (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    achievement_id TEXT NOT NULL UNIQUE,
    unlocked_at    INTEGER NOT NULL
);
```

#### Scenario: Duplicate achievement insert is a no-op
- **WHEN** `RecordAchievement` is called twice with the same achievement_id
- **THEN** the second call returns nil without inserting a duplicate row
