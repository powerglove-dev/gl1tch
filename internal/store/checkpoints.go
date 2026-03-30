package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// StepCheckpoint is the persisted write-ahead record for a single pipeline step.
// It captures the step's lifecycle state so that a pipeline can be resumed with
// full ExecutionContext re-hydration after orcai is restarted.
type StepCheckpoint struct {
	ID         int64
	RunID      int64
	StepID     string
	StepIndex  int
	Status     string // pending | running | done | failed | paused_clarify
	Prompt     string
	Output     string
	Model      string
	VarsJSON   string // flat map[string]string JSON snapshot
	StartedAt  *int64 // unix millis, nil until started
	FinishedAt *int64 // unix millis, nil until finished
	DurationMs *int64 // nil until finished
}

// StartStepCheckpoint writes a 'running' checkpoint before a step executes.
// Uses INSERT OR REPLACE so repeated calls (e.g. on retry) are idempotent.
func (s *Store) StartStepCheckpoint(runID int64, stepID string, stepIndex int, prompt, model string) error {
	now := time.Now().UnixMilli()
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO step_checkpoints
				(run_id, step_id, step_index, status, prompt, model, vars_json, started_at)
			VALUES (?, ?, ?, 'running', ?, ?, '{}', ?)`,
			runID, stepID, stepIndex, prompt, model, now,
		)
		return err
	})
}

// CompleteStepCheckpoint updates the checkpoint with step outcome on completion.
// vars may be nil — stored as '{}' in that case.
func (s *Store) CompleteStepCheckpoint(runID int64, stepID, status, output string, vars map[string]string, durationMs int64) error {
	varsJSON := "{}"
	if vars != nil {
		if b, err := json.Marshal(vars); err == nil {
			varsJSON = string(b)
		}
	}
	now := time.Now().UnixMilli()
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE step_checkpoints
			    SET status = ?, output = ?, vars_json = ?, finished_at = ?, duration_ms = ?
			  WHERE run_id = ? AND step_id = ?`,
			status, output, varsJSON, now, durationMs,
			runID, stepID,
		)
		return err
	})
}

// PauseStepCheckpoint marks a step as paused while waiting for clarification.
func (s *Store) PauseStepCheckpoint(runID int64, stepID string) error {
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE step_checkpoints SET status = 'paused_clarify' WHERE run_id = ? AND step_id = ?`,
			runID, stepID,
		)
		return err
	})
}

// LoadStepCheckpoints returns all checkpoints for a run ordered by step_index ascending.
func (s *Store) LoadStepCheckpoints(runID int64) ([]StepCheckpoint, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, step_id, step_index, status, prompt, output, model, vars_json,
		        started_at, finished_at, duration_ms
		   FROM step_checkpoints
		  WHERE run_id = ?
		  ORDER BY step_index ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []StepCheckpoint
	for rows.Next() {
		var cp StepCheckpoint
		if err := rows.Scan(
			&cp.ID, &cp.RunID, &cp.StepID, &cp.StepIndex,
			&cp.Status, &cp.Prompt, &cp.Output, &cp.Model, &cp.VarsJSON,
			&cp.StartedAt, &cp.FinishedAt, &cp.DurationMs,
		); err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return checkpoints, nil
}
