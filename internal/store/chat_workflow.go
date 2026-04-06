package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ChatWorkflow is a saved composable workflow authored in the desktop builder.
// StepsJSON is an opaque JSON blob owned by the desktop frontend so the schema
// stays stable as the builder evolves.
type ChatWorkflow struct {
	ID          int64
	WorkspaceID string
	Name        string
	StepsJSON   string
	CreatedAt   int64
	UpdatedAt   int64
}

// InsertChatWorkflow saves a new chat workflow and returns the generated ID.
func (s *Store) InsertChatWorkflow(_ context.Context, w ChatWorkflow) (int64, error) {
	now := time.Now().Unix()
	var id int64
	err := s.writer.send(func(db *sql.DB) error {
		res, err := db.Exec(
			`INSERT INTO chat_workflows (workspace_id, name, steps_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			w.WorkspaceID, w.Name, w.StepsJSON, now, now,
		)
		if err != nil {
			return fmt.Errorf("store: insert chat workflow: %w", err)
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// UpdateChatWorkflow updates an existing chat workflow's name and steps.
func (s *Store) UpdateChatWorkflow(_ context.Context, id int64, name, stepsJSON string) error {
	now := time.Now().Unix()
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE chat_workflows SET name = ?, steps_json = ?, updated_at = ? WHERE id = ?`,
			name, stepsJSON, now, id,
		)
		if err != nil {
			return fmt.Errorf("store: update chat workflow: %w", err)
		}
		return nil
	})
}

// ListChatWorkflows returns all chat workflows for a workspace, newest first.
// Returns a non-nil empty slice when there are no workflows.
func (s *Store) ListChatWorkflows(ctx context.Context, workspaceID string) ([]ChatWorkflow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, name, steps_json, created_at, updated_at
		   FROM chat_workflows
		  WHERE workspace_id = ?
		  ORDER BY updated_at DESC, id DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list chat workflows: %w", err)
	}
	defer rows.Close()

	out := []ChatWorkflow{}
	for rows.Next() {
		var w ChatWorkflow
		if err := rows.Scan(&w.ID, &w.WorkspaceID, &w.Name, &w.StepsJSON, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan chat workflow: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetChatWorkflow returns a single chat workflow by ID.
// Returns sql.ErrNoRows if not found.
func (s *Store) GetChatWorkflow(ctx context.Context, id int64) (ChatWorkflow, error) {
	var w ChatWorkflow
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, steps_json, created_at, updated_at
		   FROM chat_workflows
		  WHERE id = ?`,
		id,
	).Scan(&w.ID, &w.WorkspaceID, &w.Name, &w.StepsJSON, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return w, err
	}
	return w, nil
}

// DeleteChatWorkflow removes a chat workflow by ID.
func (s *Store) DeleteChatWorkflow(_ context.Context, id int64) error {
	return s.writer.send(func(db *sql.DB) error {
		_, err := db.Exec(`DELETE FROM chat_workflows WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("store: delete chat workflow: %w", err)
		}
		return nil
	})
}
