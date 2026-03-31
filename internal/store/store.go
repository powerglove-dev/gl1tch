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

// StoreWriter is the interface satisfied by *Store for recording run lifecycle
// events. Callers that only need to write (not query) should depend on this
// interface rather than *Store directly.
type StoreWriter interface {
	RecordRunStart(kind, name, metadata string) (int64, error)
	RecordRunComplete(id int64, exitStatus int, stdout, stderr string) error
	RecordStepComplete(ctx context.Context, runID int64, step StepRecord) error
}

// Run represents a recorded pipeline or agent run.
type Run struct {
	ID         int64
	Kind       string // "pipeline" | "agent"
	Name       string
	StartedAt  int64  // unix millis
	FinishedAt *int64 // nil if in-flight
	ExitStatus *int   // nil if in-flight
	Stdout     string
	Stderr     string
	Metadata   string       // JSON blob
	Steps      []StepRecord // per-step records, populated from the steps column
}

// Store manages the SQLite result database.
type Store struct {
	db     *sql.DB
	writer *writer
	cfg    RetentionConfig
}

// Open opens or creates the store at ~/.local/share/orcai/orcai.db.
// It enables WAL mode and applies the schema migration.
func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("store: resolve home dir: %w", err)
	}
	path := filepath.Join(home, ".local", "share", "orcai", "orcai.db")
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
func (s *Store) RecentBrainNotes(ctx context.Context, runID int64, limit int) ([]BrainNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_id, created_at, tags, body
		   FROM brain_notes
		  WHERE run_id = ?
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
// paused. It marks them as interrupted: exit_status=2, stderr="interrupted: orcai
// closed while running". Returns the IDs of the rows that were updated.
func (s *Store) RecoverOrphanedRuns() ([]int64, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`
		UPDATE runs
		SET finished_at = ?, exit_status = 2,
		    stderr = 'interrupted: orcai closed while running'
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
		  AND stderr = 'interrupted: orcai closed while running'
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

// Close shuts down the writer goroutine and closes the database.
func (s *Store) Close() error {
	s.writer.close()
	return s.db.Close()
}
