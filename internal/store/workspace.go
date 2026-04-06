package store

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// Workspace represents a chat workspace with its own set of monitored directories.
type Workspace struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Directories []string `json:"directories"` // absolute paths
	RepoNames   []string `json:"repo_names"`  // filepath.Base of each dir
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// WorkspaceMessage is a persisted chat message within a workspace.
type WorkspaceMessage struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
	BlocksJSON  string `json:"blocks_json"`
	Timestamp   int64  `json:"timestamp"`
}

// CreateWorkspace creates a new workspace and returns it.
func (s *Store) CreateWorkspace(ctx context.Context, title string, ts int64) (Workspace, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, title, ts, ts)
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return Workspace{ID: id, Title: title, CreatedAt: ts, UpdatedAt: ts}, nil
}

// ListWorkspaces returns all workspaces ordered by most recently updated.
func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, created_at, updated_at FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.Title, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, err
		}
		// Load directories for this workspace
		ws.Directories, ws.RepoNames, _ = s.getWorkspaceDirectories(ctx, ws.ID)
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}

// GetWorkspace returns a single workspace by ID.
func (s *Store) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	var ws Workspace
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at FROM workspaces WHERE id = ?`, id,
	).Scan(&ws.ID, &ws.Title, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	ws.Directories, ws.RepoNames, _ = s.getWorkspaceDirectories(ctx, ws.ID)
	return ws, nil
}

// DeleteWorkspace removes a workspace and all its directories and messages.
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_messages WHERE workspace_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM workspace_directories WHERE workspace_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	return err
}

// UpdateWorkspaceTitle updates the title and updated_at timestamp.
func (s *Store) UpdateWorkspaceTitle(ctx context.Context, id, title string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET title = ?, updated_at = ? WHERE id = ?`, title, ts, id)
	return err
}

// TouchWorkspace updates the updated_at timestamp.
func (s *Store) TouchWorkspace(ctx context.Context, id string, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`, ts, id)
	return err
}

// AddWorkspaceDirectory associates a directory with a workspace.
func (s *Store) AddWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	repoName := filepath.Base(path)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO workspace_directories (workspace_id, path, repo_name) VALUES (?, ?, ?)`,
		workspaceID, path, repoName)
	return err
}

// RemoveWorkspaceDirectory removes a directory association from a workspace.
func (s *Store) RemoveWorkspaceDirectory(ctx context.Context, workspaceID, path string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_directories WHERE workspace_id = ? AND path = ?`,
		workspaceID, path)
	return err
}

func (s *Store) getWorkspaceDirectories(ctx context.Context, workspaceID string) ([]string, []string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, repo_name FROM workspace_directories WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var dirs, repos []string
	for rows.Next() {
		var path, repo string
		if err := rows.Scan(&path, &repo); err != nil {
			return nil, nil, err
		}
		dirs = append(dirs, path)
		repos = append(repos, repo)
	}
	return dirs, repos, rows.Err()
}

// SaveWorkspaceMessage persists a chat message.
func (s *Store) SaveWorkspaceMessage(ctx context.Context, msg WorkspaceMessage) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO workspace_messages (id, workspace_id, role, blocks_json, timestamp) VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.WorkspaceID, msg.Role, msg.BlocksJSON, msg.Timestamp)
	return err
}

// GetWorkspaceMessages returns all messages for a workspace, oldest first.
func (s *Store) GetWorkspaceMessages(ctx context.Context, workspaceID string) ([]WorkspaceMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, role, blocks_json, timestamp FROM workspace_messages WHERE workspace_id = ? ORDER BY timestamp ASC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []WorkspaceMessage
	for rows.Next() {
		var m WorkspaceMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Role, &m.BlocksJSON, &m.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
