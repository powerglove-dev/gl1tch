// Package store provides a SQLite-backed result store that captures pipeline
// and agent run results with configurable retention policies.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register the sqlite driver
)

// ScoreEvent records the token usage and XP earned for a single pipeline run.
type ScoreEvent struct {
	ID                  int64
	RunID               int64
	XP                  int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	CostUSD             float64
	Provider            string
	Model               string
	CreatedAt           int64
}

// UserScore holds the player's cumulative game state.
type UserScore struct {
	TotalXP     int64
	Level       int
	StreakDays  int
	LastRunDate string
	TotalRuns   int64
}

// ProviderScore aggregates XP and run count per provider.
type ProviderScore struct {
	Provider  string
	TotalXP   int64
	TotalRuns int64
}

// StepRecord describes the persisted outcome of a single pipeline step.
type StepRecord struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	Model      string         `json:"model,omitempty"`
	Prompt     string         `json:"prompt,omitempty"`      // prompt sent to the model
	StartedAt  string         `json:"started_at,omitempty"`  // RFC3339
	FinishedAt string         `json:"finished_at,omitempty"` // RFC3339
	DurationMs int64          `json:"duration_ms,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
}

// GameStats holds aggregate behavioral statistics over a time window, used by
// the game tuner to calibrate achievement thresholds and XP formula weights.
type GameStats struct {
	TotalRuns             int64
	AvgOutputRatio        float64
	P50OutputRatio        float64
	P90OutputRatio        float64
	AvgCacheReadTokens    float64
	P90CacheReadTokens    float64
	AvgCostUSD            float64
	ProviderRunCounts     map[string]int64
	UnlockedAchievementIDs []string
	StepFailureRate       float64
	RunsSinceDate         int64
}

// StoreWriter is the interface satisfied by *Store for recording run lifecycle
// events. Callers that only need to write (not query) should depend on this
// interface rather than *Store directly.
type StoreWriter interface {
	RecordRunStart(kind, name, metadata string) (int64, error)
	RecordRunComplete(id int64, exitStatus int, stdout, stderr string) error
	RecordStepComplete(ctx context.Context, runID int64, step StepRecord) error
}

// Run represents a recorded pipeline or agent run.
//
// WorkspaceID identifies the workspace whose chain bar (or
// equivalent execution surface) produced the run. Empty means
// "global / unattributed" — used for legacy rows from before the
// workspace-scoped split and for runs from CLI tools that don't
// have a workspace context.
type Run struct {
	ID          int64
	Kind        string // "pipeline" | "agent"
	Name        string
	WorkspaceID string
	StartedAt   int64  // unix millis
	FinishedAt  *int64 // nil if in-flight
	ExitStatus  *int   // nil if in-flight
	Stdout      string
	Stderr      string
	Metadata    string       // JSON blob
	Steps       []StepRecord // per-step records, populated from the steps column
}

// Store manages the SQLite result database.
type Store struct {
	db     *sql.DB
	writer *writer
	cfg    RetentionConfig
}

// Open opens or creates the store at ~/.local/share/glitch/glitch.db.
// It enables WAL mode and applies the schema migration.
func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("store: resolve home dir: %w", err)
	}
	path := filepath.Join(home, ".local", "share", "glitch", "glitch.db")
	return OpenAt(path)
}

// OpenAt opens or creates the store at the given path.
// It enables WAL mode and applies the schema migration.
// This function exists primarily for testing with t.TempDir().
func OpenAt(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir: %w", err)
	}

	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}

	if err := applySchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}

	cfg := loadRetentionConfig()

	s := &Store{
		db:     db,
		writer: newWriter(db),
		cfg:    cfg,
	}
	return s, nil
}

// BrainNote is a single note written to the brain_notes table during a pipeline run.
type BrainNote struct {
	ID        int64
	RunID     int64
	StepID    string
	CreatedAt int64
	Tags      string
	Body      string
}

// InsertBrainNote inserts a brain note into the brain_notes table and returns
// the new row's ID.
func (s *Store) InsertBrainNote(ctx context.Context, note BrainNote) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO brain_notes (run_id, step_id, created_at, tags, body) VALUES (?, ?, ?, ?, ?)`,
		note.RunID, note.StepID, note.CreatedAt, note.Tags, note.Body,
	)
	if err != nil {
		return 0, fmt.Errorf("store: insert brain note: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert brain note last id: %w", err)
	}
	return id, nil
}

// RecentBrainNotes returns up to limit brain notes for runID, ordered by
// created_at descending (most recent first).
// Capability notes (tags LIKE 'type:capability%') are excluded — they live in
// a separate query path via CapabilityNotes to avoid count interference.
func (s *Store) RecentBrainNotes(ctx context.Context, runID int64, limit int) ([]BrainNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_id, created_at, tags, body
		   FROM brain_notes
		  WHERE run_id = ?
		    AND tags NOT LIKE 'type:capability%'
		  ORDER BY created_at DESC
		  LIMIT ?`,
		runID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: recent brain notes: %w", err)
	}
	defer rows.Close()

	var notes []BrainNote
	for rows.Next() {
		var n BrainNote
		if err := rows.Scan(&n.ID, &n.RunID, &n.StepID, &n.CreatedAt, &n.Tags, &n.Body); err != nil {
			return nil, fmt.Errorf("store: recent brain notes scan: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: recent brain notes rows: %w", err)
	}
	if notes == nil {
		notes = []BrainNote{}
	}
	return notes, nil
}

// AllBrainNotes returns all brain notes across all runs, ordered by created_at descending.
func (s *Store) AllBrainNotes(ctx context.Context) ([]BrainNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_id, created_at, tags, body
		   FROM brain_notes
		  ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: all brain notes: %w", err)
	}
	defer rows.Close()

	var notes []BrainNote
	for rows.Next() {
		var n BrainNote
		if err := rows.Scan(&n.ID, &n.RunID, &n.StepID, &n.CreatedAt, &n.Tags, &n.Body); err != nil {
			return nil, fmt.Errorf("store: all brain notes scan: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: all brain notes rows: %w", err)
	}
	if notes == nil {
		notes = []BrainNote{}
	}
	return notes, nil
}

// UpsertBrainNote inserts a brain note preserving its original ID, ignoring
// the row if that ID already exists. Used during backup restore.
func (s *Store) UpsertBrainNote(ctx context.Context, note BrainNote) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO brain_notes (id, run_id, step_id, created_at, tags, body) VALUES (?, ?, ?, ?, ?, ?)`,
		note.ID, note.RunID, note.StepID, note.CreatedAt, note.Tags, note.Body,
	)
	if err != nil {
		return fmt.Errorf("store: upsert brain note: %w", err)
	}
	return nil
}

// UpsertPrompt inserts a prompt preserving its original ID, ignoring the row
// if that ID already exists. Used during backup restore.
func (s *Store) UpsertPrompt(ctx context.Context, p Prompt) error {
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO prompts (id, title, body, model_slug, last_response, cwd, input_format, output_format, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Title, p.Body, p.ModelSlug, p.LastResponse, p.CWD, p.InputFormat, p.OutputFormat, p.CreatedAt, p.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("store: upsert prompt: %w", err)
		}
		return nil
	})
}

// CapabilityNotes returns all brain notes that are system-level capability entries
// (run_id=0 AND tags LIKE 'type:capability%'), ordered by created_at ascending.
// Returns an empty non-nil slice when none exist.
func (s *Store) CapabilityNotes(ctx context.Context) ([]BrainNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_id, created_at, tags, body
		   FROM brain_notes
		  WHERE run_id = 0
		    AND tags LIKE 'type:capability%'
		  ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: capability notes: %w", err)
	}
	defer rows.Close()

	notes := []BrainNote{}
	for rows.Next() {
		var n BrainNote
		if err := rows.Scan(&n.ID, &n.RunID, &n.StepID, &n.CreatedAt, &n.Tags, &n.Body); err != nil {
			return nil, fmt.Errorf("store: capability notes scan: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: capability notes rows: %w", err)
	}
	return notes, nil
}

// UpsertCapabilityNote inserts or updates a system-level capability brain note
// keyed by (run_id=0, step_id). On conflict, updates body, tags, and created_at.
// Idempotent: calling with the same step_id twice will update, not duplicate.
// Implemented at the application layer (SELECT then INSERT or UPDATE) to avoid
// requiring a unique constraint on brain_notes(run_id, step_id).
func (s *Store) UpsertCapabilityNote(ctx context.Context, note BrainNote) error {
	// Check for an existing capability note with run_id=0 and same step_id.
	var existingID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM brain_notes WHERE run_id = 0 AND step_id = ? LIMIT 1`,
		note.StepID,
	).Scan(&existingID)

	switch {
	case err == sql.ErrNoRows:
		// No existing row — insert.
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO brain_notes (run_id, step_id, created_at, tags, body) VALUES (0, ?, ?, ?, ?)`,
			note.StepID, note.CreatedAt, note.Tags, note.Body,
		)
		if err != nil {
			return fmt.Errorf("store: upsert capability note insert: %w", err)
		}
	case err != nil:
		return fmt.Errorf("store: upsert capability note lookup: %w", err)
	default:
		// Existing row found — update it.
		_, err = s.db.ExecContext(ctx,
			`UPDATE brain_notes SET body = ?, tags = ?, created_at = ? WHERE id = ?`,
			note.Body, note.Tags, note.CreatedAt, existingID,
		)
		if err != nil {
			return fmt.Errorf("store: upsert capability note update: %w", err)
		}
	}
	return nil
}

// UpdateBrainNote updates the body and tags of an existing brain note by ID.
func (s *Store) UpdateBrainNote(ctx context.Context, id int64, body, tags string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE brain_notes SET body = ?, tags = ? WHERE id = ?`,
		body, tags, id,
	)
	if err != nil {
		return fmt.Errorf("store: update brain note: %w", err)
	}
	return nil
}

// RecoverOrphanedRuns finds runs with finished_at=NULL and exit_status=NULL
// that do NOT have a pending (unanswered) clarification — those are legitimately
// paused. It marks them as interrupted: exit_status=2, stderr="interrupted: glitch
// closed while running". Returns the IDs of the rows that were updated.
func (s *Store) RecoverOrphanedRuns() ([]int64, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`
		UPDATE runs
		SET finished_at = ?, exit_status = 2,
		    stderr = 'interrupted: glitch closed while running'
		WHERE finished_at IS NULL
		  AND exit_status IS NULL
		  AND id NOT IN (
		      SELECT CAST(run_id AS INTEGER) FROM clarifications WHERE answer IS NULL
		  )`, now)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT id FROM runs
		WHERE exit_status = 2
		  AND stderr = 'interrupted: glitch closed while running'
		  AND finished_at = ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id) //nolint:errcheck
		ids = append(ids, id)
	}
	return ids, nil
}

// DB returns the underlying *sql.DB for direct read queries.
// Write operations should go through RecordRunStart/RecordRunComplete
// to benefit from the serialized write queue.
func (s *Store) DB() *sql.DB { return s.db }

// RecordScoreEvent inserts a new score_events row.
func (s *Store) RecordScoreEvent(ctx context.Context, e ScoreEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO score_events (run_id, xp, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, cost_usd, provider, model, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.RunID, e.XP, e.InputTokens, e.OutputTokens, e.CacheReadTokens, e.CacheCreationTokens,
		e.CostUSD, e.Provider, e.Model, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: record score event: %w", err)
	}
	return nil
}

// GetUserScore returns the player's current score, ensuring the singleton row
// exists (id=1) before reading it.
func (s *Store) GetUserScore(ctx context.Context) (UserScore, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_score (id, total_xp, level, streak_days, last_run_date, total_runs) VALUES (1, 0, 1, 0, '', 0)`,
	)
	if err != nil {
		return UserScore{}, fmt.Errorf("store: get user score ensure row: %w", err)
	}
	var us UserScore
	row := s.db.QueryRowContext(ctx,
		`SELECT total_xp, level, streak_days, last_run_date, total_runs FROM user_score WHERE id = 1`,
	)
	if err := row.Scan(&us.TotalXP, &us.Level, &us.StreakDays, &us.LastRunDate, &us.TotalRuns); err != nil {
		return UserScore{}, fmt.Errorf("store: get user score scan: %w", err)
	}
	return us, nil
}

// UpdateUserScore upserts the singleton user_score row (id=1).
func (s *Store) UpdateUserScore(ctx context.Context, us UserScore) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO user_score (id, total_xp, level, streak_days, last_run_date, total_runs) VALUES (1, ?, ?, ?, ?, ?)`,
		us.TotalXP, us.Level, us.StreakDays, us.LastRunDate, us.TotalRuns,
	)
	if err != nil {
		return fmt.Errorf("store: update user score: %w", err)
	}
	return nil
}

// RecordAchievement records an achievement as unlocked. Idempotent via INSERT OR IGNORE.
func (s *Store) RecordAchievement(ctx context.Context, achievementID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO achievements (achievement_id, unlocked_at) VALUES (?, ?)`,
		achievementID, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("store: record achievement: %w", err)
	}
	return nil
}

// HasAchievement returns true if the given achievement ID is already recorded.
func (s *Store) HasAchievement(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM achievements WHERE achievement_id = ?`, id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("store: has achievement: %w", err)
	}
	return count > 0, nil
}

// GetUnlockedAchievements returns all unlocked achievement IDs.
func (s *Store) GetUnlockedAchievements(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT achievement_id FROM achievements ORDER BY unlocked_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: get achievements: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: get achievements scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get achievements rows: %w", err)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// PersonalBest holds a single tracked personal best metric.
type PersonalBest struct {
	Metric     string
	Value      float64
	RunID      string
	RecordedAt time.Time
}

// ICEEncounter represents a pending or resolved ICE encounter.
type ICEEncounter struct {
	ID       string
	ICEClass string
	RunID    string
	Deadline time.Time
	Resolved bool
	Outcome  string
}

// InsertOrUpdatePersonalBest replaces the stored value for metric if value is
// better (higher, except for fastest_run_ms and lowest_cost_usd which are lower).
func (s *Store) InsertOrUpdatePersonalBest(metric string, value float64, runID string) error {
	// For "lower is better" metrics, we want to keep the smallest value.
	lowerIsBetter := metric == "fastest_run_ms" || metric == "lowest_cost_usd"

	var existing float64
	err := s.db.QueryRow(`SELECT value FROM game_personal_bests WHERE metric = ?`, metric).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("store: personal best lookup: %w", err)
	}
	if err == nil {
		// Row exists — only update if this is better.
		if lowerIsBetter && value >= existing {
			return nil
		}
		if !lowerIsBetter && value <= existing {
			return nil
		}
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO game_personal_bests (metric, value, run_id, recorded_at) VALUES (?, ?, ?, ?)`,
		metric, value, runID, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("store: update personal best: %w", err)
	}
	return nil
}

// GetPersonalBests returns all personal best rows ordered by metric name.
func (s *Store) GetPersonalBests() ([]PersonalBest, error) {
	rows, err := s.db.Query(`SELECT metric, value, run_id, recorded_at FROM game_personal_bests ORDER BY metric`)
	if err != nil {
		return nil, fmt.Errorf("store: get personal bests: %w", err)
	}
	defer rows.Close()
	var bests []PersonalBest
	for rows.Next() {
		var pb PersonalBest
		var recordedAtMS int64
		if err := rows.Scan(&pb.Metric, &pb.Value, &pb.RunID, &recordedAtMS); err != nil {
			return nil, fmt.Errorf("store: get personal bests scan: %w", err)
		}
		pb.RecordedAt = time.UnixMilli(recordedAtMS)
		bests = append(bests, pb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get personal bests rows: %w", err)
	}
	return bests, nil
}

// InsertICEEncounter records a new ICE encounter with the given deadline.
func (s *Store) InsertICEEncounter(id, iceClass, runID string, deadline time.Time) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO ice_encounters (id, ice_class, run_id, deadline, resolved) VALUES (?, ?, ?, ?, 0)`,
		id, iceClass, runID, deadline.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("store: insert ice encounter: %w", err)
	}
	return nil
}

// GetPendingICEEncounter returns the most recent unresolved encounter, or nil if none.
func (s *Store) GetPendingICEEncounter() (*ICEEncounter, error) {
	var enc ICEEncounter
	var deadlineMS int64
	var outcome sql.NullString
	err := s.db.QueryRow(
		`SELECT id, ice_class, run_id, deadline, resolved, outcome FROM ice_encounters WHERE resolved = 0 ORDER BY deadline DESC LIMIT 1`,
	).Scan(&enc.ID, &enc.ICEClass, &enc.RunID, &deadlineMS, &enc.Resolved, &outcome)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get pending ice encounter: %w", err)
	}
	enc.Deadline = time.UnixMilli(deadlineMS)
	enc.Outcome = outcome.String
	return &enc, nil
}

// ResolveICEEncounter marks an encounter as resolved with the given outcome.
func (s *Store) ResolveICEEncounter(id, outcome string) error {
	_, err := s.db.Exec(
		`UPDATE ice_encounters SET resolved = 1, outcome = ? WHERE id = ?`,
		outcome, id,
	)
	if err != nil {
		return fmt.Errorf("store: resolve ice encounter: %w", err)
	}
	return nil
}

// AutoResolveExpiredEncounters finds unresolved encounters past their deadline
// and marks them as losses. applyPenalty is called once per expired encounter.
func (s *Store) AutoResolveExpiredEncounters(applyPenalty func()) error {
	now := time.Now().UnixMilli()
	rows, err := s.db.Query(
		`SELECT id FROM ice_encounters WHERE resolved = 0 AND deadline < ?`, now,
	)
	if err != nil {
		return fmt.Errorf("store: auto-resolve ice encounters query: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	_ = rows.Err()

	for _, id := range ids {
		if _, err := s.db.Exec(
			`UPDATE ice_encounters SET resolved = 1, outcome = 'loss' WHERE id = ?`, id,
		); err != nil {
			continue
		}
		if applyPenalty != nil {
			applyPenalty()
		}
	}
	return nil
}

// ScoreEventsByProvider aggregates XP and run count grouped by provider.
func (s *Store) ScoreEventsByProvider(ctx context.Context) (map[string]ProviderScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, SUM(xp), COUNT(*) FROM score_events GROUP BY provider`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: score events by provider: %w", err)
	}
	defer rows.Close()
	result := make(map[string]ProviderScore)
	for rows.Next() {
		var ps ProviderScore
		if err := rows.Scan(&ps.Provider, &ps.TotalXP, &ps.TotalRuns); err != nil {
			return nil, fmt.Errorf("store: score events by provider scan: %w", err)
		}
		result[ps.Provider] = ps
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: score events by provider rows: %w", err)
	}
	return result, nil
}

// Close shuts down the writer goroutine and closes the database.
func (s *Store) Close() error {
	s.writer.close()
	return s.db.Close()
}
