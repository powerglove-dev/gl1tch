package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// writeOp is a single serialized write operation sent through the writer channel.
type writeOp struct {
	fn  func(*sql.DB) error
	res chan error
}

// writer serializes all writes to the database through a single goroutine,
// preventing SQLite write contention.
type writer struct {
	ch   chan writeOp
	done chan struct{}
}

// newWriter starts the writer goroutine and returns a writer.
func newWriter(db *sql.DB) *writer {
	w := &writer{
		ch:   make(chan writeOp, 64),
		done: make(chan struct{}),
	}
	go w.loop(db)
	return w
}

// loop processes write operations sequentially.
func (w *writer) loop(db *sql.DB) {
	defer close(w.done)
	for op := range w.ch {
		op.res <- op.fn(db)
	}
}

// send sends fn to the writer goroutine and blocks until it completes.
func (w *writer) send(fn func(*sql.DB) error) error {
	res := make(chan error, 1)
	w.ch <- writeOp{fn: fn, res: res}
	return <-res
}

// close stops the writer goroutine gracefully.
func (w *writer) close() {
	close(w.ch)
	<-w.done
}

// RecordRunStart inserts a new in-flight run row and returns its ID.
// started_at is recorded in unix milliseconds.
// metadata is an optional JSON blob (pass "" to omit).
//
// Equivalent to RecordRunStartWithWorkspace with workspaceID="".
// Kept as a convenience wrapper for callers (chiefly the legacy
// pipeline runner path) that don't yet have a workspace id to thread.
func (s *Store) RecordRunStart(kind, name, metadata string) (int64, error) {
	return s.RecordRunStartWithWorkspace(kind, name, metadata, "")
}

// RecordRunStartWithWorkspace is the workspace-aware variant of
// RecordRunStart. The workspace_id is stamped on the row so the
// PipelineIndexer can scope its query to one workspace's runs and
// avoid cross-workspace contamination in glitch-pipelines.
//
// Empty workspaceID is allowed and means "global / unattributed",
// preserving the legacy behavior for callers that don't have a
// workspace context.
func (s *Store) RecordRunStartWithWorkspace(kind, name, metadata, workspaceID string) (int64, error) {
	startedAt := time.Now().UnixMilli()
	var id int64
	err := s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`INSERT INTO runs (kind, name, started_at, metadata, workspace_id) VALUES (?, ?, ?, ?, ?)`,
			kind, name, startedAt, metadata, workspaceID,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RecordRunComplete updates the run row with exit status, stdout, stderr,
// and calls AutoPrune with the configured retention settings.
func (s *Store) RecordRunComplete(id int64, exitStatus int, stdout, stderr string) error {
	finishedAt := time.Now().UnixMilli()
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE runs
			    SET finished_at = ?,
			        exit_status = ?,
			        stdout      = ?,
			        stderr      = ?
			  WHERE id = ?`,
			finishedAt, exitStatus, stdout, stderr, id,
		)
		if err != nil {
			return err
		}
		return autoPruneDB(db, s.cfg.MaxAgeDays, s.cfg.MaxRows)
	})
}

// RecordStepComplete upserts step into the steps JSON array of the run
// identified by runID. If a step with the same ID already exists it is
// replaced; otherwise the new record is appended. The operation is serialized
// through the store writer to prevent concurrent write contention.
func (s *Store) RecordStepComplete(_ context.Context, runID int64, step StepRecord) error {
	return s.writer.send(func(db *sql.DB) error {
		// Read current steps JSON.
		var raw sql.NullString
		if err := db.QueryRow(`SELECT steps FROM runs WHERE id = ?`, runID).Scan(&raw); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("store: record step complete: run %d not found", runID)
			}
			return fmt.Errorf("store: record step complete: %w", err)
		}

		// Parse existing records.
		var steps []StepRecord
		if raw.Valid && raw.String != "" {
			if err := json.Unmarshal([]byte(raw.String), &steps); err != nil {
				// Corrupt data — start fresh.
				steps = nil
			}
		}

		// Upsert: replace existing entry with matching ID or append.
		found := false
		for i, existing := range steps {
			if existing.ID == step.ID {
				steps[i] = step
				found = true
				break
			}
		}
		if !found {
			steps = append(steps, step)
		}

		// Marshal back and persist.
		b, err := json.Marshal(steps)
		if err != nil {
			return fmt.Errorf("store: record step complete: marshal: %w", err)
		}
		if _, err := db.Exec(`UPDATE runs SET steps = ? WHERE id = ?`, string(b), runID); err != nil {
			return fmt.Errorf("store: record step complete: %w", err)
		}
		return nil
	})
}
